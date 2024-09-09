package tracker

import (
	"fmt"
	"testing"

	"github.com/satmihir/fair/pkg/testutils"
)

func TestFairnessTrackerError(t *testing.T) {
	origErr := fmt.Errorf("original error")
	dataErr := NewFairnessTrackerError(origErr, "data error occurred")

	testutils.TestError(t, &FairnessTrackerError{}, dataErr, "data error occurred: original error", origErr)
}
