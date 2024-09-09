package data

import (
	"github.com/satmihir/fair/pkg/utils"
)

type DataError struct {
	*utils.BaseError
}

func NewDataError(wrapped error, msg string, args ...any) *DataError {
	return &DataError{
		BaseError: utils.NewBaseError(wrapped, msg, args...),
	}
}
