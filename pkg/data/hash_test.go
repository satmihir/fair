package data

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/satmihir/fair/pkg/config"
)

// Vectors from the SipHash reference implementation: the key is bytes
// 00..0f and the input of length n is bytes 00..n-1.
func TestSipHash24ReferenceVectors(t *testing.T) {
	const k0, k1 = 0x0706050403020100, 0x0f0e0d0c0b0a0908
	input := make([]byte, 16)
	for i := range input {
		input[i] = byte(i)
	}
	for n, want := range map[int]uint64{
		0:  0x726fdb47dd0e0e31,
		1:  0x74f839c593dc67fd,
		2:  0x0d6c8009d9a94f5a,
		3:  0x85676696d7fb7e2d,
		4:  0xcf2794e0277187b7,
		5:  0x18765564cd99a68d,
		6:  0xcbc9466e58fee3ce,
		7:  0xab0200f58b01d137,
		8:  0x93f5f5799a932462,
		9:  0x9e0082df0ba9e4b0,
		15: 0xa129ca6149be45e5, // The worked example in the SipHash paper.
	} {
		require.Equalf(t, want, sipHash24(k0, k1, input[:n]), "length %d", n)
	}
}

func TestSipHash24DependsOnKeyAndEveryByte(t *testing.T) {
	id := []byte("tenant-7f3a9c1e-4b2d-4e8a-9c41-2f6d8a1b3c5e")
	base := sipHash24(1, 2, id)
	require.Equal(t, base, sipHash24(1, 2, id))
	require.NotEqual(t, base, sipHash24(3, 2, id))
	require.NotEqual(t, base, sipHash24(1, 3, id))
	for i := range id {
		changed := append([]byte(nil), id...)
		changed[i] ^= 1
		require.NotEqualf(t, base, sipHash24(1, 2, changed), "byte %d", i)
	}
	require.NotEqual(t, base, sipHash24(1, 2, id[:len(id)-1]))
}

func bucketRow(t *testing.T, s *Structure, id []byte) string {
	t.Helper()
	return fmt.Sprint(s.RegisterRequest(context.Background(), id).ResultStats.BucketIndexes)
}

func hashTestConfig(levels, buckets uint32) config.FairnessTrackerConfig {
	return config.FairnessTrackerConfig{
		L: levels, M: buckets, Pi: .5, Pd: .25,
		FinalProbabilityFunction: config.MinFinalProbabilityFunction,
	}
}

// fixedKeys returns a reproducible key source, so that a placement test can
// never fail on a legitimate chance collision under some freshly drawn key.
func fixedKeys(seed int64) *rand.Rand {
	return rand.New(rand.NewSource(seed))
}

func hashTestStructure(t *testing.T, levels, buckets uint32, keys *rand.Rand) *Structure {
	t.Helper()
	cfg := hashTestConfig(levels, buckets)
	cfg.HashKeySource = keys
	s, err := NewStructure(withStats(&cfg), 1)
	require.NoError(t, err)
	return s
}

// These two identifiers have the same MurmurHash3 x64 hash under every seed,
// so with the previous hash they shared every bucket in every generation and
// one could get the other throttled. Under a secret key they are unrelated.
// The keys are fixed, so this outcome is the same on every run.
func TestSeedIndependentMurmurCollisionNoLongerSharesBuckets(t *testing.T) {
	victim := []byte("tenant-7f3a9c1e-4b2d-4e8a-9c41-2f6d8a1b3c5e")
	crafted, err := hex.DecodeString("14c5703fca00d86ea526ea0b264ac3e434623264b0e696b5612d396334312d326636643861316233633565")
	require.NoError(t, err)
	require.Len(t, crafted, len(victim))

	keys := fixedKeys(1)
	for generation := 0; generation < 50; generation++ {
		s := hashTestStructure(t, 3, 1000, keys)
		require.NotEqual(t, bucketRow(t, s, victim), bucketRow(t, s, crafted), "generation %d", generation)
	}
}

// Count pairs of clients that share a bucket at every level. Clients with
// different hashes should do so about pairs/M^L times; indexes derived
// linearly from two hash halves gave about pairs/M^2 however many levels
// there were. The keys are fixed, so the counts are the same on every run.
func TestClientsRarelyShareEveryLevel(t *testing.T) {
	const clients = 20000
	const pairs = clients * (clients - 1) / 2

	keys := fixedKeys(2)
	sharedRows := func(levels, buckets uint32) int {
		s := hashTestStructure(t, levels, buckets, keys)
		seen := make(map[string]int, clients)
		for i := 0; i < clients; i++ {
			seen[bucketRow(t, s, []byte(fmt.Sprintf("client-%d", i)))]++
		}
		shared := 0
		for _, n := range seen {
			shared += n * (n - 1) / 2
		}
		return shared
	}

	// Two levels: about 191 expected. Five standard deviations either way
	// also catches levels that fail to spread clients at all.
	expected := float64(pairs) / (1024 * 1024)
	got := float64(sharedRows(2, 1024))
	require.InDelta(t, expected, got, 5*math.Sqrt(expected), "two levels")

	// Five levels: about 2e-7 expected by chance, where the linear derivation
	// still produced about 191. Power-of-two and other widths behaved
	// differently before, so check both.
	require.Zero(t, sharedRows(5, 1024), "five levels, power-of-two width")
	require.Zero(t, sharedRows(5, 1000), "five levels")
}

// Every bucket of every level should receive its share of clients.
func TestLevelIndexIsUniformAndInRange(t *testing.T) {
	const samples = 200000
	for _, buckets := range []uint32{1, 2, 7, 64, 1000} {
		for level := uint32(0); level < 4; level++ {
			counts := make([]int, buckets)
			for i := uint64(0); i < samples; i++ {
				// Sequential hashes are the hardest input for a weak mixer.
				index := levelIndex(i, level, buckets)
				if index >= buckets {
					t.Fatalf("width %d level %d: index %d out of range", buckets, level, index)
				}
				counts[index]++
			}
			mean := float64(samples) / float64(buckets)
			if buckets == 1 {
				require.Equal(t, samples, counts[0])
				continue
			}
			for b, n := range counts {
				require.InDeltaf(t, mean, float64(n), 6*math.Sqrt(mean), "width %d level %d bucket %d", buckets, level, b)
			}
		}
	}
}

// Two identifiers with the same 64-bit hash agree at every level: the levels
// are remixes of one value, not independent hashes.
func TestEqualHashesShareEveryLevel(t *testing.T) {
	for _, hash := range []uint64{0, 1, math.MaxUint64, 0x0123456789abcdef} {
		for level := uint32(0); level < 8; level++ {
			require.Equal(t, levelIndex(hash, level, 1024), levelIndex(hash, level, 1024))
		}
	}

	// The same key places an identifier the same way in any structure.
	a := hashTestStructure(t, 6, 4096, fixedKeys(3))
	b := hashTestStructure(t, 6, 4096, fixedKeys(3))
	require.Equal(t, a.hashKey, b.hashKey)
	require.Equal(t, bucketRow(t, a, []byte("client")), bucketRow(t, b, []byte("client")))
}

// By default every structure reads its own key from crypto/rand.
func TestFreshKeysComeFromCryptoRand(t *testing.T) {
	cfg := hashTestConfig(4, 4096)
	require.Nil(t, cfg.HashKeySource)
	seen := make(map[[2]uint64]bool)
	for i := 0; i < 32; i++ {
		s, err := NewStructure(withStats(&cfg), 1)
		require.NoError(t, err)
		require.NotEqual(t, [2]uint64{}, s.hashKey)
		require.False(t, seen[s.hashKey], "a 128-bit key repeated")
		seen[s.hashKey] = true
	}
}

// Keys must not follow math/rand's package-level generator, which rand.Seed
// or GODEBUG=randautoseed=0 can make repeat across process starts.
func TestFreshKeysIgnoreGlobalMathRandSeed(t *testing.T) {
	cfg := hashTestConfig(4, 4096)
	draw := func() [2]uint64 {
		//nolint:staticcheck // Deliberately pin the generator the keys must not use.
		rand.Seed(1)
		s, err := NewStructure(withStats(&cfg), 1)
		require.NoError(t, err)
		return s.hashKey
	}
	require.NotEqual(t, draw(), draw())
}

func TestInjectedKeySourceIsReadSixteenBytesAtATime(t *testing.T) {
	keys := fixedKeys(4)
	first := hashTestStructure(t, 4, 4096, keys)
	second := hashTestStructure(t, 4, 4096, keys)
	require.NotEqual(t, first.hashKey, second.hashKey, "each structure takes the next key")

	// A reader that returns one byte per call must still fill the key.
	cfg := hashTestConfig(4, 4096)
	cfg.HashKeySource = iotest.OneByteReader(fixedKeys(4))
	slow, err := NewStructure(withStats(&cfg), 1)
	require.NoError(t, err)
	require.Equal(t, first.hashKey, slow.hashKey)
}

func TestKeyGenerationFailureFailsConstruction(t *testing.T) {
	errEntropy := errors.New("entropy unavailable")
	for name, source := range map[string]io.Reader{
		"error":      iotest.ErrReader(errEntropy),
		"short read": &limitedSource{remaining: 15, err: errEntropy},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := hashTestConfig(4, 4096)
			cfg.HashKeySource = source
			s, err := NewStructure(withStats(&cfg), 1)
			require.Nil(t, s)
			var dataErr *DataError
			require.ErrorAs(t, err, &dataErr)
			require.ErrorIs(t, err, errEntropy)
		})
	}
}

// limitedSource runs dry one byte before a whole key has been read.
type limitedSource struct {
	remaining int
	err       error
}

func (l *limitedSource) Read(p []byte) (int, error) {
	if l.remaining == 0 {
		return 0, l.err
	}
	n := min(len(p), l.remaining)
	l.remaining -= n
	return n, nil
}
