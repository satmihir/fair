package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateStructConfig(t *testing.T) {
	config := &StructureConfig{
		L: 0,
	}

	err := validateStructureConfig(config)
	assert.Error(t, err)

	config = &StructureConfig{
		L: 1,
		M: 0,
	}

	err = validateStructureConfig(config)
	assert.Error(t, err)

	config = &StructureConfig{
		L:  1,
		M:  1,
		Pd: 0,
		Pi: 0,
	}

	err = validateStructureConfig(config)
	assert.Error(t, err)

	config = &StructureConfig{
		L:  1,
		M:  1,
		Pd: 10,
		Pi: 10,
	}

	err = validateStructureConfig(config)
	assert.Error(t, err)

	config = &StructureConfig{
		L:  1,
		M:  1,
		Pd: .15,
		Pi: .1,
	}

	err = validateStructureConfig(config)
	assert.Error(t, err)

	config = &StructureConfig{
		L:  1,
		M:  1,
		Pd: .1,
		Pi: .15,
	}

	err = validateStructureConfig(config)
	assert.NoError(t, err)
}

func TestNewStructureFailsValidation(t *testing.T) {
	config := &StructureConfig{
		L:  1,
		M:  1,
		Pd: .15,
		Pi: .1,
	}
	_, err := NewStructure(config)
	assert.Error(t, err)
}

func TestNewStructure(t *testing.T) {
	config := &StructureConfig{
		L:  2,
		M:  24,
		Pd: .1,
		Pi: .15,
	}
	structure, err := NewStructure(config)
	assert.NoError(t, err)
	assert.NotNil(t, structure)

	assert.Equal(t, len(structure.levels), 2)
	assert.Equal(t, len(structure.levels[0]), 24)
}

func TestHashes(t *testing.T) {
	datum := []byte("hello world")
	hashes := GenerateNHashesUsing64Bit(datum, 3)

	assert.Equal(t, len(hashes), 3)

	hashes2 := GenerateNHashesUsing64Bit(datum, 3)

	assert.Equal(t, hashes[0], hashes2[0])
	assert.Equal(t, hashes[1], hashes2[1])
	assert.Equal(t, hashes[2], hashes2[2])
}
