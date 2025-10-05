package serialization

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Serializer handles serialization and deserialization of FairStruct
type Serializer struct{}

// NewSerializer creates a new serializer instance
func NewSerializer() *Serializer {
	return &Serializer{}
}

// Serialize converts a FairStruct to bytes
func (s *Serializer) Serialize(fairStruct *FairStruct) ([]byte, error) {
	if fairStruct == nil {
		return nil, fmt.Errorf("fairStruct cannot be nil")
	}

	data, err := proto.Marshal(fairStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal FairStruct: %w", err)
	}

	return data, nil
}

// Deserialize converts bytes back to a FairStruct
func (s *Serializer) Deserialize(data []byte) (*FairStruct, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("data cannot be empty")
	}

	fairStruct := &FairStruct{}
	err := proto.Unmarshal(data, fairStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal FairStruct: %w", err)
	}

	return fairStruct, nil
}

// SerializeToJSON converts a FairStruct to JSON bytes
func (s *Serializer) SerializeToJSON(fairStruct *FairStruct) ([]byte, error) {
	if fairStruct == nil {
		return nil, fmt.Errorf("fairStruct cannot be nil")
	}

	// Use protojson for JSON serialization
	return protojson.Marshal(fairStruct)
}

// DeserializeFromJSON converts JSON bytes back to a FairStruct
func (s *Serializer) DeserializeFromJSON(data []byte) (*FairStruct, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("data cannot be empty")
	}

	fairStruct := &FairStruct{}
	err := protojson.Unmarshal(data, fairStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal FairStruct from JSON: %w", err)
	}

	return fairStruct, nil
}
