package serialization

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
			t.Logf("Original TrackerId: %s, Deserialized TrackerId: %s", fairStruct.Cfg.TrackerId, deserializedStruct.Cfg.TrackerId)
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
			t.Logf("Original TrackerId: %s, Deserialized TrackerId: %s", fairStruct.Cfg.TrackerId, deserializedStruct.Cfg.TrackerId)
			t.Logf("Original Lambda: %f, Deserialized Lambda: %f", fairStruct.Cfg.Lambda, deserializedStruct.Cfg.Lambda)
			t.Logf("Original HostGuid: %s, Deserialized HostGuid: %s", fairStruct.Meta.HostGuid, deserializedStruct.Meta.HostGuid)
		}
	})
}

func TestSerializerErrorPaths(t *testing.T) {
	serializer := NewSerializer()

	t.Run("Serialize nil input returns errNil", func(t *testing.T) {
		data, err := serializer.Serialize(nil)
		require.Nil(t, data)
		require.ErrorIs(t, err, errNil)
	})

	t.Run("Serialize invalid UTF-8 returns marshal error", func(t *testing.T) {
		fairStruct := createSampleFairStruct()
		fairStruct.Cfg.TrackerId = string([]byte{0xff, 0xfe, 0xfd})

		data, err := serializer.Serialize(fairStruct)
		require.Nil(t, data)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to marshal FairStruct")
	})

	t.Run("Deserialize empty data returns errEmptyData", func(t *testing.T) {
		out, err := serializer.Deserialize(nil)
		require.Nil(t, out)
		require.ErrorIs(t, err, errEmptyData)
	})

	t.Run("Deserialize invalid bytes returns wrapped unmarshal error", func(t *testing.T) {
		out, err := serializer.Deserialize([]byte{0xff, 0x00, 0x01, 0x02})
		require.Nil(t, out)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to unmarshal FairStruct")
	})

	t.Run("SerializeToJSON nil input returns errNil", func(t *testing.T) {
		data, err := serializer.SerializeToJSON(nil)
		require.Nil(t, data)
		require.ErrorIs(t, err, errNil)
	})

	t.Run("SerializeToJSON invalid UTF-8 returns error", func(t *testing.T) {
		fairStruct := createSampleFairStruct()
		fairStruct.Meta.HostGuid = string([]byte{0xff, 0xfe, 0xfd})

		data, err := serializer.SerializeToJSON(fairStruct)
		require.Nil(t, data)
		require.Error(t, err)
	})

	t.Run("DeserializeFromJSON empty data returns errEmptyData", func(t *testing.T) {
		out, err := serializer.DeserializeFromJSON(nil)
		require.Nil(t, out)
		require.ErrorIs(t, err, errEmptyData)
	})

	t.Run("DeserializeFromJSON invalid JSON returns wrapped unmarshal error", func(t *testing.T) {
		out, err := serializer.DeserializeFromJSON([]byte(`{"cfg":`))
		require.NotNil(t, out)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to unmarshal FairStruct from JSON")
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

	randomUint64 := func(max uint64) uint64 {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
		return uint64(n.Int64())
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
			TrackerId:         fmt.Sprintf("%d", randomInt64(10000)+1),
			ConfigVersion:     randomUint64(100) + 1,
			M:                 randomUint32(1000) + 1000,
			L:                 randomUint32(50) + 5,
			Pi:                randomFloat(),
			Pd:                randomFloat() * 0.5,      // Keep Pd smaller than Pi typically
			Lambda:            0.8 + randomFloat()*0.19, // 0.8-0.99 range
			RotationFrequency: durationpb.New(randomDuration()),
			LevelSquashFn:     LevelSquashingFunction(randomInt64(2)), // 0 or 1
		},
		Data: &FairRuntimeData{
			Runtime: &FairRunParameters{
				AlgoParams: &AlgoParams{
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
