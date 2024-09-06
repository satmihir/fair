package data

import (
	"fmt"
	"testing"

	"github.com/satmihir/fair/pkg/testutils"
)

func TestDataError(t *testing.T) {
	origErr := fmt.Errorf("original error")
	dataErr := NewDataError(origErr, "data error occurred")

	testutils.TestError(t, &DataError{}, dataErr, "data error occurred: original error", origErr)
}
