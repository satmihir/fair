package utils

import "fmt"

// A general base error for all package errors
type BaseError struct {
	msg          string
	wrappedError error
}

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
