package logger

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type testLogger struct{}

func newTestLogger() *testLogger {
	return &testLogger{}
}

func (t *testLogger) Printf(format string, args ...any) {
	fmt.Printf(format, args...)
}

func (t *testLogger) Print(args ...any) {
	fmt.Print(args...)
}

func (t *testLogger) Println(args ...any) {
	fmt.Println(args...)
}

func (t *testLogger) Fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	panic(msg)
}

type captureLogger struct {
	printfCalled bool
	printfFmt    string
	printfArgs   []any
	fatalfCalled bool
	fatalfFmt    string
	fatalfArgs   []any
}

func (c *captureLogger) Printf(format string, args ...any) {
	c.printfCalled = true
	c.printfFmt = format
	c.printfArgs = append([]any(nil), args...)
}

func (c *captureLogger) Print(_ ...any) {}

func (c *captureLogger) Println(_ ...any) {}

func (c *captureLogger) Fatalf(format string, args ...any) {
	c.fatalfCalled = true
	c.fatalfFmt = format
	c.fatalfArgs = append([]any(nil), args...)
}

func TestStdLogger_Print_VariousInputs(t *testing.T) {
	tests := []struct {
		name     string
		inputs   []any
		expected string
	}{
		{
			name:     "single string",
			inputs:   []any{"hello"},
			expected: "fair: hello\n",
		},
		{
			name:     "multiple args",
			inputs:   []any{"hello", "world", 123},
			expected: "fair: helloworld123\n",
		},
		{
			name:     "empty args",
			inputs:   []any{},
			expected: "fair: \n",
		},
		{
			name:     "nil arg",
			inputs:   []any{nil},
			expected: "fair: <nil>\n",
		},
		{
			name:     "mixed types",
			inputs:   []any{"string", 42, true, 3.14},
			expected: "fair: string42 true 3.14\n",
		},
		{
			name:     "special characters",
			inputs:   []any{"hello\nworld", "tab\there"},
			expected: "fair: hello\nworldtab\there\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			buf := &bytes.Buffer{}
			logger := &stdLogger{
				l: log.New(buf, "fair: ", 0),
			}

			// Act
			logger.Print(tt.inputs...)

			// Assert
			output := buf.String()
			require.Equal(t, output, tt.expected)
		})
	}
}

func TestStdLogger_Printf_VariousFormats(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []any
		expected string
	}{
		{
			name:     "simple string",
			format:   "hello %s",
			args:     []any{"world"},
			expected: "fair: hello world\n",
		},
		{
			name:     "multiple format specifiers",
			format:   "user %s has %d points",
			args:     []any{"alice", 42},
			expected: "fair: user alice has 42 points\n",
		},
		{
			name:     "no format specifiers",
			format:   "static message",
			args:     []any{},
			expected: "fair: static message\n",
		},
		{
			name:     "integer formatting",
			format:   "number: %d, hex: %x",
			args:     []any{255, 255},
			expected: "fair: number: 255, hex: ff\n",
		},
		{
			name:     "float formatting",
			format:   "pi: %.2f",
			args:     []any{3.14159},
			expected: "fair: pi: 3.14\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			buf := &bytes.Buffer{}
			logger := &stdLogger{
				l: log.New(buf, "fair: ", 0),
			}

			// Act
			logger.Printf(tt.format, tt.args...)

			// Assert
			output := buf.String()
			require.Equal(t, output, tt.expected)
		})
	}
}

func TestStdLogger_Println_VariousInputs(t *testing.T) {
	tests := []struct {
		name     string
		inputs   []any
		expected string
	}{
		{
			name:     "single string with newline",
			inputs:   []any{"hello"},
			expected: "fair: hello\n",
		},
		{
			name:     "multiple args with newline",
			inputs:   []any{"hello", "world"},
			expected: "fair: hello world\n",
		},
		{
			name:     "empty args with newline",
			inputs:   []any{},
			expected: "fair: \n",
		},
		{
			name:     "nil arg with newline",
			inputs:   []any{nil},
			expected: "fair: <nil>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			buf := &bytes.Buffer{}
			logger := &stdLogger{
				l: log.New(buf, "fair: ", 0),
			}

			// Act
			logger.Println(tt.inputs...)

			// Assert
			output := buf.String()
			require.Equal(t, output, tt.expected)
		})
	}
}

func TestPrint_ConcurrentAccess(t *testing.T) {
	// Arrange
	oldStderr := os.Stderr
	reader, writer, _ := os.Pipe()
	os.Stderr = writer
	defer func() {
		writer.Close()
		os.Stderr = oldStderr
	}()

	const numGoroutines = 10
	const messagesPerGoroutine = 100

	logger := &stdLogger{
		l: log.New(writer, "fair: ", 0),
	}

	// Act
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				logger.Printf("goroutine-%d-message-%d", id, j)
			}
		}(i)
	}

	wg.Wait()
	writer.Close()

	// Assert
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	expectedTotal := numGoroutines * messagesPerGoroutine
	require.Equal(t, expectedTotal, len(lines))

	// Verify all expected messages are present (order doesn't matter for concurrency test)
	messageCount := make(map[string]int)
	for _, line := range lines {
		messageCount[line]++
	}

	// Check that each expected message appears exactly once
	for i := 0; i < numGoroutines; i++ {
		for j := 0; j < messagesPerGoroutine; j++ {
			expectedMsg := fmt.Sprintf("fair: goroutine-%d-message-%d", i, j)
			require.Equal(t, 1, messageCount[expectedMsg], "Message %s should appear exactly once", expectedMsg)
		}
	}
}

func TestSetLogger(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		l        Logger
		validate func(t *testing.T)
	}{
		{
			name: "switching with stdout logger",
			l:    newTestLogger(),
			validate: func(t *testing.T) {
				t.Helper()
				oldStdout := os.Stdout
				testLog := "testing if logs are working"

				reader, writer, _ := os.Pipe()
				os.Stdout = writer

				Print(testLog)

				writer.Close()
				os.Stdout = oldStdout

				var buf bytes.Buffer
				if _, err := buf.ReadFrom(reader); err != nil {
					t.Fatalf("failed to read: %v", err)
				}
				output := buf.String()

				if output != testLog {
					t.Fatalf("test failed, expected: %s, found: %s", testLog, output)
				}
			},
		},
		{
			name: "passing a nil logger",
			l:    nil,
			validate: func(t *testing.T) {
				t.Helper()
				oldStdout := os.Stdout
				testLog := "testing if logs are working"

				reader, writer, _ := os.Pipe()
				os.Stdout = writer

				Print(testLog)

				writer.Close()
				os.Stdout = oldStdout

				var buf bytes.Buffer
				if _, err := buf.ReadFrom(reader); err != nil {
					t.Fatalf("failed to read: %v", err)
				}
				output := buf.String()

				if len(output) > 0 {
					t.Fatalf("test failed, expected nothing but found: %s", output)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetLogger(tt.l)
			tt.validate(t)
		})
	}
}

func TestNoOpLoggerMethods(t *testing.T) {
	n := &noOpLogger{}
	require.NotPanics(t, func() { n.Printf("x=%d", 1) })
	require.NotPanics(t, func() { n.Print("hello") })
	require.NotPanics(t, func() { n.Println("hello") })
	require.NotPanics(t, func() { n.Fatalf("fatal: %s", "noop") })
}

func TestNewStdLogger(t *testing.T) {
	l := NewStdLogger()
	require.NotNil(t, l)
	_, ok := l.(*stdLogger)
	require.True(t, ok)
}

func TestGlobalPrintfUsesConfiguredLogger(t *testing.T) {
	prev := GetLogger()
	cl := &captureLogger{}
	SetLogger(cl)
	t.Cleanup(func() {
		SetLogger(prev)
	})

	Printf("value=%d", 7)

	require.True(t, cl.printfCalled)
	require.Equal(t, "value=%d", cl.printfFmt)
	require.Equal(t, []any{7}, cl.printfArgs)
}

func TestGlobalFatalfUsesConfiguredLogger(t *testing.T) {
	prev := GetLogger()
	cl := &captureLogger{}
	SetLogger(cl)
	t.Cleanup(func() {
		SetLogger(prev)
	})

	Fatalf("problem=%s", "x")

	require.True(t, cl.fatalfCalled)
	require.Equal(t, "problem=%s", cl.fatalfFmt)
	require.Equal(t, []any{"x"}, cl.fatalfArgs)
}

func TestStdLoggerFatalfUsesInjectedFunc(t *testing.T) {
	prevFatalf := stdLoggerFatalf
	t.Cleanup(func() {
		stdLoggerFatalf = prevFatalf
	})

	var called bool
	stdLoggerFatalf = func(_ *log.Logger, format string, args ...any) {
		called = true
		require.Equal(t, "fatal=%s", format)
		require.Equal(t, []any{"boom"}, args)
	}

	s := &stdLogger{
		l: log.New(os.Stderr, "fair: ", 0),
	}
	s.Fatalf("fatal=%s", "boom")
	require.True(t, called)
}

func TestDefaultStdLoggerFatalfCallsExit(t *testing.T) {
	prevExit := stdLoggerExit
	t.Cleanup(func() {
		stdLoggerExit = prevExit
	})

	var exitCode int
	stdLoggerExit = func(code int) {
		exitCode = code
		panic("exit-called")
	}

	require.PanicsWithValue(t, "exit-called", func() {
		s := &stdLogger{
			l: log.New(io.Discard, "fair: ", 0),
		}
		s.Fatalf("fatal=%s", "boom")
	})
	require.Equal(t, 1, exitCode)
}
