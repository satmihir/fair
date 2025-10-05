package serialization

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestSerialization(t *testing.T) {
	// Create a sample FairStruct
	fairStruct := createSampleFairStruct()

	// Create serializer
	serializer := NewSerializer()

	// Test binary serialization
	t.Run("Binary Serialization", func(t *testing.T) {
		// Serialize to binary
		binaryData, err := serializer.Serialize(fairStruct)
		if err != nil {
			t.Fatalf("Failed to serialize: %v", err)
		}

		// Deserialize from binary
		deserializedStruct, err := serializer.Deserialize(binaryData)
		if err != nil {
			t.Fatalf("Failed to deserialize: %v", err)
		}

		// Verify the data with deep comparison
		if !proto.Equal(fairStruct, deserializedStruct) {
			t.Errorf("Binary serialization/deserialization did not preserve exact contents")
			t.Logf("Original TrackerId: %d, Deserialized TrackerId: %d", fairStruct.Cfg.TrackerId, deserializedStruct.Cfg.TrackerId)
			t.Logf("Original Pi: %f, Deserialized Pi: %f", fairStruct.Cfg.Pi, deserializedStruct.Cfg.Pi)
			t.Logf("Original bucket count: %d, Deserialized bucket count: %d",
				len(fairStruct.Data.Data.Levels[0].Buckets), len(deserializedStruct.Data.Data.Levels[0].Buckets))
		}
	})

	// Test JSON serialization
	t.Run("JSON Serialization", func(t *testing.T) {
		// Serialize to JSON
		jsonData, err := serializer.SerializeToJSON(fairStruct)
		if err != nil {
			t.Fatalf("Failed to serialize to JSON: %v", err)
		}

		// Print JSON for inspection
		t.Logf("JSON output: %s", string(jsonData))

		// Deserialize from JSON
		deserializedStruct, err := serializer.DeserializeFromJSON(jsonData)
		if err != nil {
			t.Fatalf("Failed to deserialize from JSON: %v", err)
		}

		// Verify the data with deep comparison
		if !proto.Equal(fairStruct, deserializedStruct) {
			t.Errorf("JSON serialization/deserialization did not preserve exact contents")
			t.Logf("Original TrackerId: %d, Deserialized TrackerId: %d", fairStruct.Cfg.TrackerId, deserializedStruct.Cfg.TrackerId)
			t.Logf("Original Lambda: %f, Deserialized Lambda: %f", fairStruct.Cfg.Lambda, deserializedStruct.Cfg.Lambda)
			t.Logf("Original HostGuid: %s, Deserialized HostGuid: %s", fairStruct.Meta.HostGuid, deserializedStruct.Meta.HostGuid)
		}
	})
}

// createSampleFairStruct creates a sample FairStruct for testing
func createSampleFairStruct() *FairStruct {
	// Generate random values
	randomInt64 := func(max int64) int64 {
		n, _ := rand.Int(rand.Reader, big.NewInt(max))
		return n.Int64()
	}

	randomUint32 := func(max uint32) uint32 {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
		return uint32(n.Int64())
	}

	randomFloat := func() float64 {
		n, _ := rand.Int(rand.Reader, big.NewInt(1000))
		return float64(n.Int64()) / 1000.0
	}

	randomDuration := func() time.Duration {
		minutes := randomInt64(30) + 1 // 1-30 minutes
		return time.Duration(minutes) * time.Minute
	}

	return &FairStruct{
		Cfg: &TrackerCfg{
			TrackerId:         randomInt64(10000) + 1,
			ConfigVersion:     randomInt64(100) + 1,
			M:                 randomUint32(1000) + 50,
			L:                 randomUint32(50) + 5,
			Pi:                randomFloat(),
			Pd:                randomFloat() * 0.5,      // Keep Pd smaller than Pi typically
			Lambda:            0.8 + randomFloat()*0.19, // 0.8-0.99 range
			RotationFrequency: durationpb.New(randomDuration()),
			LevelSquashFn:     LevelSquashingFunction(randomInt64(2)), // 0 or 1
		},
		Data: &FairRuntimeData{
			Runtime: &FairRunParameters{
				Algoparams: &AlgoParams{
					Algorithm:  Algorithm_MURMURHASH_32,
					MurmurSeed: randomUint32(1000000),
				},
			},
			Data: &FairData{
				Levels: []*Level{
					{
						Buckets: []*Bucket{
							{
								Probability:       randomFloat(),
								LastUpdatedTimeMs: uint64(time.Now().UnixMilli() - randomInt64(3600000)), // Random time in last hour
							},
							{
								Probability:       randomFloat(),
								LastUpdatedTimeMs: uint64(time.Now().UnixMilli() - randomInt64(3600000)),
							},
							{
								Probability:       randomFloat(),
								LastUpdatedTimeMs: uint64(time.Now().UnixMilli() - randomInt64(3600000)),
							},
						},
					},
					{
						Buckets: []*Bucket{
							{
								Probability:       randomFloat(),
								LastUpdatedTimeMs: uint64(time.Now().UnixMilli() - randomInt64(7200000)), // Random time in last 2 hours
							},
						},
					},
				},
			},
		},
		Meta: &HostMeta{
			HostGuid:       fmt.Sprintf("host-%d-%d-%d", randomInt64(1000), randomInt64(1000), randomInt64(1000)),
			SerializedAtMs: uint64(time.Now().UnixMilli()),
		},
	}
}
