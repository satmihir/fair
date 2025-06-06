package testutils

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestError verifies that an error implements the expected behavior, message and
// wrapped error. It is a helper used across unit tests.
func TestError(t *testing.T, expectedType interface{}, errInstance error, expectedMessage string, wrappedErr error) {
	t.Helper()

	_, ok := errInstance.(interface{ Unwrap() error })

	assert.True(t, ok, "Error should be of expected type")
	assert.Equal(t, expectedMessage, errInstance.Error(), "Error message should match")
	assert.Equal(t, wrappedErr, errors.Unwrap(errInstance), "Wrapped error should match the expected original error")
}
