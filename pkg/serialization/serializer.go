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

var errEmptyData error = fmt.Errorf("data cannot be empty")
var errNil error = fmt.Errorf("fairStruct cannot be nil")

// Serialize converts a FairStruct to bytes
func (s *Serializer) Serialize(fairStruct *FairStruct) (result []byte, err error) {
	if fairStruct == nil {
		err = errNil
		return
	}

	data, err := proto.Marshal(fairStruct)
	if err != nil {
		err = fmt.Errorf("failed to marshal FairStruct: %w", err)
		return
	}

	return data, nil
}

// Deserialize converts bytes back to a FairStruct
func (s *Serializer) Deserialize(data []byte) (ft *FairStruct, err error) {
	if len(data) == 0 {
		err = errEmptyData
		return
	}

	fairStruct := &FairStruct{}
	err1 := proto.Unmarshal(data, fairStruct)
	if err1 != nil {
		err = fmt.Errorf("failed to unmarshal FairStruct: %w", err1)
		return nil, err
	}

	return fairStruct, nil
}

// SerializeToJSON converts a FairStruct to JSON bytes
func (s *Serializer) SerializeToJSON(fairStruct *FairStruct) (stuff []byte, err error) {
	if fairStruct == nil {
		err = errNil
		return
	}

	// Use protojson for JSON serialization
	stuff, err = protojson.Marshal(fairStruct)
	return
}

// DeserializeFromJSON converts JSON bytes back to a FairStruct
func (s *Serializer) DeserializeFromJSON(data []byte) (fst *FairStruct, err error) {
	if len(data) == 0 {
		err = errEmptyData
		return
	}

	fst = &FairStruct{}
	err1 := protojson.Unmarshal(data, fst)
	if err1 != nil {
		err = fmt.Errorf("failed to unmarshal FairStruct from JSON: %w", err1)
		return
	}

	return
}
