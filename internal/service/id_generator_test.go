// Copyright 2024 The MinURL Authors

package service //nolint:testpackage // White-box tests

import (
	"testing"

	"github.com/bits-and-blooms/bitset"
)

func TestFeistelCollisionSample(t *testing.T) {
	const sampleSize = 1_000_000 // Test 1 million values to detect collisions quickly

	generator := NewDefaultFeistelIDGenerator()

	// Use bitset to track permuted values. Allocate space for all uint32 values.
	seen := bitset.New(0)

	for i := uint32(0); i < sampleSize; i++ {
		v := generator.permuted(i)

		if seen.Test(uint(v)) {
			t.Fatalf("collision detected at sequence %d (permuted value: %d)", i, v)
		}

		seen.Set(uint(v))
	}
}
