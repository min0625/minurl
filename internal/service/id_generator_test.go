// Copyright 2024 The MinURL Authors

package service //nolint:testpackage // White-box tests

import (
	"testing"
)

func TestFeistelCollisionSample(t *testing.T) {
	const sampleSize = 1_000_000 // Test 1 million values to detect collisions quickly

	generator := NewDefaultFeistelIDGenerator()

	// Track permuted values using a map to avoid memory issues with bitset.
	// The permuted output is a full uint32 value from the Feistel permutation,
	// not bounded by sampleSize.
	seen := make(map[uint32]bool, sampleSize)

	for i := range uint32(sampleSize) {
		v := generator.permuted(i)

		if seen[v] {
			t.Fatalf("collision detected at sequence %d (permuted value: %d)", i, v)
		}

		seen[v] = true
	}
}
