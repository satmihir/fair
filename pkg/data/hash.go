package data

import (
	"encoding/binary"
	"math/bits"
)

// sipHash24 computes SipHash-2-4 of data under the 128-bit key (k0, k1).
//
// SipHash is a keyed pseudorandom function: without the key, a client cannot
// predict where an identifier lands or construct two identifiers that land
// together. Unkeyed-by-design hashes such as MurmurHash3 have collisions that
// hold under every seed, which would let one client share another's buckets
// in every generation.
func sipHash24(k0, k1 uint64, data []byte) uint64 {
	v0 := k0 ^ 0x736f6d6570736575
	v1 := k1 ^ 0x646f72616e646f6d
	v2 := k0 ^ 0x6c7967656e657261
	v3 := k1 ^ 0x7465646279746573

	// The final block carries the input length in its top byte.
	last := uint64(len(data)) << 56

	for ; len(data) >= 8; data = data[8:] {
		m := binary.LittleEndian.Uint64(data)
		v3 ^= m
		v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
		v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
		v0 ^= m
	}

	for i, b := range data {
		last |= uint64(b) << (8 * i)
	}
	v3 ^= last
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0 ^= last

	v2 ^= 0xff
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)

	return v0 ^ v1 ^ v2 ^ v3
}

func sipRound(v0, v1, v2, v3 uint64) (uint64, uint64, uint64, uint64) {
	v0 += v1
	v1 = bits.RotateLeft64(v1, 13)
	v1 ^= v0
	v0 = bits.RotateLeft64(v0, 32)
	v2 += v3
	v3 = bits.RotateLeft64(v3, 16)
	v3 ^= v2
	v0 += v3
	v3 = bits.RotateLeft64(v3, 21)
	v3 ^= v0
	v2 += v1
	v1 = bits.RotateLeft64(v1, 17)
	v1 ^= v2
	v2 = bits.RotateLeft64(v2, 32)
	return v0, v1, v2, v3
}

// levelIndex picks the bucket for one level from a client's 64-bit hash.
//
// Each level remixes the hash with the SplitMix64 output function, stepping
// by its odd "golden gamma" constant. Two clients with different hashes then
// agree at every level only about as often as unrelated draws would, roughly
// 1/M^L. The levels are not independent hashes, though: all of them come from
// this one 64-bit value, so two identifiers with the same hash agree at every
// level, which for an ideal 64-bit hash happens for one pair in 2^64. That
// floor is what bounds the shared-row rate once M^L exceeds 2^64.
//
// Deriving the indexes linearly instead (index = h1 + level*h2) leaves only
// two degrees of freedom: two clients that agree on h1 and h2 modulo the
// level width share a bucket at every level, at a rate near 1/M^2 no matter
// how many levels are configured.
func levelIndex(hash uint64, level, buckets uint32) uint32 {
	z := hash + (uint64(level)+1)*0x9e3779b97f4a7c15
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	return uint32(z % uint64(buckets))
}
