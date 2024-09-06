package testutils

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError(t *testing.T, expectedType interface{}, errInstance error, expectedMessage string, wrappedErr error) {
	_, ok := errInstance.(interface{ Unwrap() error })

	assert.True(t, ok, "Error should be of expected type")
	assert.Equal(t, expectedMessage, errInstance.Error(), "Error message should match")
	assert.Equal(t, wrappedErr, errors.Unwrap(errInstance), "Wrapped error should match the expected original error")
}
