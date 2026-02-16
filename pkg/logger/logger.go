package logger

import (
	"log"
	"os"
	"sync"
)

type Logger interface {
	Printf(format string, args ...any)
	Print(args ...any)
	Println(args ...any)
	Fatalf(format string, args ...any)
}

type noOpLogger struct{}

func (n *noOpLogger) Printf(_ string, _ ...any) {}
func (n *noOpLogger) Print(_ ...any)            {}
func (n *noOpLogger) Println(_ ...any)          {}
func (n *noOpLogger) Fatalf(_ string, _ ...any) {}

var (
	defaultNoOpLogger        = &noOpLogger{}
	logger            Logger = defaultNoOpLogger
	mx                sync.RWMutex
	stdLoggerExit     = os.Exit
	stdLoggerFatalf   = func(l *log.Logger, format string, args ...any) {
		l.Printf(format, args...)
		stdLoggerExit(1)
	}
)

type stdLogger struct {
	l *log.Logger
}

func NewStdLogger() Logger {
	return &stdLogger{
		l: log.New(os.Stderr, "fair: ", log.LstdFlags),
	}
}

func (s *stdLogger) Printf(format string, args ...any) {
	s.l.Printf(format, args...)
}

func (s *stdLogger) Print(args ...any) {
	s.l.Print(args...)
}

func (s *stdLogger) Println(args ...any) {
	s.l.Println(args...)
}

func (s *stdLogger) Fatalf(format string, args ...any) {
	stdLoggerFatalf(s.l, format, args...)
}

// Replaces default logger with provided logger
// in case of nil logger, it resets to default no-op logger
func SetLogger(l Logger) {
	mx.Lock()
	defer mx.Unlock()
	if l == nil {
		logger = defaultNoOpLogger
		return
	}
	logger = l
}

// Returns currently configured logger
func GetLogger() Logger {
	mx.RLock()
	defer mx.RUnlock()
	return logger
}

// Print uses whichever logger is currently set
func Print(args ...any) {
	GetLogger().Print(args...)
}

// Printf uses whichever logger is currently set
func Printf(format string, args ...any) {
	GetLogger().Printf(format, args...)
}

// Fatalf uses whichever logger is currently set and panics after logging
func Fatalf(format string, args ...any) {
	GetLogger().Fatalf(format, args...)
}
