package data

import (
	"github.com/satmihir/fair/pkg/utils"
)

// DataError represents an error related to the underlying data structure used by
// the tracker.
type DataError struct {
	*utils.BaseError
}

// NewDataError wraps the given error with additional context for data layer
// issues.
func NewDataError(wrapped error, msg string, args ...any) *DataError {
	return &DataError{
		BaseError: utils.NewBaseError(wrapped, msg, args...),
	}
}
