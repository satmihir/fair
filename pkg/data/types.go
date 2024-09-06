package data

import "github.com/satmihir/fair/pkg/utils"

// The config for the underlying data structure. Largely for internal use.
type StructureConfig struct {
	// Size of the row at each level
	M uint32
	// Number of levels in the structure
	L uint32
	// The delta P to add to a bucket's probability when there's an error
	Pi float64
	// The delta P to subtract from a bucket's probability when there's a success
	Pd float64
}

// The data struecture interface of SBF
type IStructure interface{}

// The umbrella error for this package
type DataError struct {
	*utils.BaseError
}

func NewDataError(wrapped error, msg string, args ...any) *DataError {
	return &DataError{
		BaseError: utils.NewBaseError(wrapped, msg, args...),
	}
}
