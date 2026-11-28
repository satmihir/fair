package config

import "fmt"

// MaxRotationBytes is the per-tracker budget for estimated peak bucket storage.
// It applies to tuned and manual configurations alike, not to process-wide RSS.
const MaxRotationBytes = 64 << 20

// Allow for two resident generations plus the replacement constructed before
// promotion. The charges include padding above the current bucket, pointer,
// row slice, and fixed structure sizes; revisit them if the layout changes.
const (
	generations = 3
	fixedBytes  = 4096
	rowBytes    = 64
	bucketBytes = 64
)

func checkRotationBudget(levels, buckets uint32) error {
	// One row fits in uint64 even at MaxUint32 buckets. Divide the remaining
	// budget before comparing levels: multiplying first can wrap at large L*M.
	perRow := rowBytes + uint64(buckets)*bucketBytes
	if uint64(levels) > (MaxRotationBytes/generations-fixedBytes)/perRow {
		return fmt.Errorf("L=%d M=%d exceed the %d-byte rotation storage budget", levels, buckets, MaxRotationBytes)
	}
	return nil
}
