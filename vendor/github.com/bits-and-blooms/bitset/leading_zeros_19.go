//go:build go1.9
// +build go1.9

package bitset

import "math/bits"

func len64(v uint64) uint {
	return uint(bits.Len64(v))
}

func leadingZeroes64(v uint64) uint {
	return uint(bits.LeadingZeros64(v))
}
