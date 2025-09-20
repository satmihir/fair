package logger

import (
	"io"
	"log"
	"os"
	"sync"
)

var (
	defaultNoOpLogger        = log.New(io.Discard, "fair: ", log.LstdFlags)
	logger            Logger = defaultNoOpLogger
	mx                sync.RWMutex
)

type Logger interface {
	Printf(format string, args ...any)
	Print(args ...any)
}

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

// Printf uses whichever logger is currently
func Printf(format string, args ...any) {
	GetLogger().Printf(format, args...)
}
