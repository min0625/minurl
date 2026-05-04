// Copyright 2024 The MinURL Authors

package service //nolint:testpackage // White-box tests

import (
	"strings"
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

func TestEncodeBase58Zero(t *testing.T) {
	t.Parallel()

	// Zero must encode to the first Base58 alphabet character.
	got := encodeBase58(0)
	if got != "1" {
		t.Fatalf("encodeBase58(0) = %q, want %q", got, "1")
	}
}

func TestEncodeBase58KnownValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value uint32
		want  string
	}{
		{1, "2"},
		{57, "z"},
		{58, "21"},
		{3364, "211"}, // 58^2
	}

	for _, tt := range tests {
		got := encodeBase58(tt.value)
		if got != tt.want {
			t.Fatalf("encodeBase58(%d) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestEncodeBase58OutputIsBase58(t *testing.T) {
	t.Parallel()

	values := []uint32{0, 1, 100, 58 * 58, 1<<16 - 1, 1<<24 - 1, 1<<32 - 1}

	for _, v := range values {
		got := encodeBase58(v)

		if got == "" {
			t.Fatalf("encodeBase58(%d) returned empty string", v)
		}

		for _, ch := range got {
			if !strings.ContainsRune(base58Alphabet, ch) {
				t.Fatalf("encodeBase58(%d) = %q contains non-base58 character %q", v, got, ch)
			}
		}
	}
}

func TestDeriveFeistelKeysLength(t *testing.T) {
	t.Parallel()

	keys := deriveFeistelKeys(0)
	if len(keys) != 4 {
		t.Fatalf("deriveFeistelKeys returned %d keys, want 4", len(keys))
	}
}

func TestDeriveFeistelKeysDeterministic(t *testing.T) {
	t.Parallel()

	a := deriveFeistelKeys(42)
	b := deriveFeistelKeys(42)

	if a != b {
		t.Fatalf("deriveFeistelKeys(42) not deterministic: %v != %v", a, b)
	}
}

func TestDeriveFeistelKeysDifferentSeeds(t *testing.T) {
	t.Parallel()

	a := deriveFeistelKeys(1)
	b := deriveFeistelKeys(2)

	if a == b {
		t.Fatalf("deriveFeistelKeys produced identical keys for different seeds")
	}
}

func TestFeistelRoundDeterministic(t *testing.T) {
	t.Parallel()

	got1 := feistelRound(0xABCD, 0x1234)
	got2 := feistelRound(0xABCD, 0x1234)

	if got1 != got2 {
		t.Fatalf("feistelRound not deterministic: %d != %d", got1, got2)
	}
}

func TestFeistelRoundOutputFits16Bits(t *testing.T) {
	t.Parallel()

	inputs := []struct{ half, key uint32 }{
		{0, 0},
		{0xFFFF, 0xFFFF},
		{0xABCD, 0x1234},
		{1, 1},
	}

	for _, tt := range inputs {
		got := feistelRound(tt.half, tt.key)
		if got > feistelHalfMask {
			t.Fatalf("feistelRound(%#x, %#x) = %#x exceeds 16-bit mask", tt.half, tt.key, got)
		}
	}
}

func TestFeistelPermuteDeterministic(t *testing.T) {
	t.Parallel()

	keys := deriveFeistelKeys(999)

	for _, v := range []uint32{0, 1, 0xFFFFFFFF, 42, 1 << 16} {
		a := feistelPermute(v, keys)
		b := feistelPermute(v, keys)

		if a != b {
			t.Fatalf("feistelPermute(%d) not deterministic: %d != %d", v, a, b)
		}
	}
}

func TestGenerateReturnsNonEmptyBase58(t *testing.T) {
	t.Parallel()

	g := NewDefaultFeistelIDGenerator()

	for _, seq := range []uint32{0, 1, 100, 1000, 1<<16 - 1} {
		id := g.Generate(seq)

		if id == "" {
			t.Fatalf("Generate(%d) returned empty string", seq)
		}

		for _, ch := range id {
			if !strings.ContainsRune(base58Alphabet, ch) {
				t.Fatalf("Generate(%d) = %q contains non-base58 character %q", seq, id, ch)
			}
		}
	}
}

func TestGenerateDeterministic(t *testing.T) {
	t.Parallel()

	a := NewFeistelIDGeneratorWithSeed(7)
	b := NewFeistelIDGeneratorWithSeed(7)

	for _, seq := range []uint32{0, 1, 42, 999} {
		if gotA, gotB := a.Generate(seq), b.Generate(seq); gotA != gotB {
			t.Fatalf("Generate(%d) not deterministic for same seed: %q != %q", seq, gotA, gotB)
		}
	}
}
