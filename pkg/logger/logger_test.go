package logger

import (
	"bytes"
	"fmt"
	"os"
	"testing"
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
				oldStdout := os.Stdout
				testLog := "testing if logs are working"

				reader, writer, _ := os.Pipe()
				os.Stdout = writer

				Print(testLog)

				writer.Close()
				os.Stdout = oldStdout

				var buf bytes.Buffer
				buf.ReadFrom(reader)
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
				oldStdout := os.Stdout
				testLog := "testing if logs are working"

				reader, writer, _ := os.Pipe()
				os.Stdout = writer

				Print(testLog)

				writer.Close()
				os.Stdout = oldStdout

				var buf bytes.Buffer
				buf.ReadFrom(reader)
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
