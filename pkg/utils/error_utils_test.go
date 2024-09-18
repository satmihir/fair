package utils

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaseError(t *testing.T) {
	e := fmt.Errorf("wrapped")
	testError := NewBaseError(e, "wrapping text %s", "xyz")

	assert.Equal(t, testError.Error(), "wrapping text xyz: wrapped")
	assert.Equal(t, errors.Unwrap(testError), e)
}
