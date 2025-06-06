package utils

import "fmt"

// BaseError is a simple wrapper that implements the error interface and allows
// attaching additional context to errors returned by the library.
type BaseError struct {
	msg          string
	wrappedError error
}

// NewBaseError creates a new BaseError that wraps an underlying error with a
// formatted message.
func NewBaseError(wrapped error, msg string, args ...any) *BaseError {
	m := fmt.Sprintf(msg, args...)
	return &BaseError{
		msg:          m,
		wrappedError: wrapped,
	}
}

func (be *BaseError) Error() string {
	if be.wrappedError != nil {
		return fmt.Sprintf("%s: %v", be.msg, be.wrappedError)
	}
	return be.msg
}

func (be *BaseError) Unwrap() error {
	return be.wrappedError
}
