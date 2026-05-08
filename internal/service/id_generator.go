// Copyright 2024 The MinURL Authors

package service

const (
	// defaultFeistelSeed is the default seed for deterministic ID generation.
	// Using a memorable hex value (0xC0FFEE = "COFFEE") for clarity in debugging.
	defaultFeistelSeed uint32 = 0xC0FFEE42

	// splitMix32Increment is the increment for SplitMix32-style key derivation.
	// This is the "golden ratio" constant: 2^32 / phi.
	splitMix32Increment uint32 = 0x9E3779B9

	// feistelHalfMask is the bitmask for 16-bit half-blocks in Feistel rounds.
	feistelHalfMask uint32 = 0xFFFF

	// feistelRoundMul is the multiplier used in Feistel round function for diffusion.
	feistelRoundMul uint32 = 0x45d9f3b

	// base58RadixSize is the size of the Base58 alphabet.
	base58RadixSize = 58

	// lowPartLen is the fixed encoded length of the low 32-bit part of a sequence.
	// base58 can encode uint32 in at most 6 characters (58^6 > 2^32).
	lowPartLen = 6
)

// IDGenerator describes short ID generation operations required by ShortURLService.
type IDGenerator interface {
	Generate(sequence uint64) string
}

// FeistelIDGenerator produces base58 short IDs from a uint64 sequence.
//
// The sequence is split into a low 32-bit part and a high 32-bit part:
//
//	id = base58Padded(feistel32(low), lowPartLen) + base58(high)
//
// When high == 0 the suffix is omitted, yielding exactly lowPartLen (6) characters.
// Each increment of high adds a tier of 2^32 extra IDs; the suffix grows by one
// character roughly every 58 tiers, keeping IDs short well beyond 2^32 entries.
type FeistelIDGenerator struct {
	keys [4]uint32
}

// NewFeistelIDGeneratorWithSeed creates an ID generator with deterministic keys from a seed.
func NewFeistelIDGeneratorWithSeed(seed uint32) *FeistelIDGenerator {
	keys := deriveFeistelKeys(seed)

	return &FeistelIDGenerator{keys: keys}
}

// NewDefaultFeistelIDGenerator creates an ID generator using the built-in default seed.
func NewDefaultFeistelIDGenerator() *FeistelIDGenerator {
	return NewFeistelIDGeneratorWithSeed(defaultFeistelSeed)
}

// permuted returns the Feistel-permuted value of the given uint32 sequence.
func (g *FeistelIDGenerator) permuted(sequence uint32) uint32 {
	return feistelPermute(sequence, g.keys)
}

// Generate converts a monotonically increasing uint64 sequence into a base58 short ID.
//
// The low 32 bits are Feistel-permuted and base58-encoded with fixed width (6 chars).
// The high 32 bits are base58-encoded without padding and appended as a suffix.
// When the sequence fits in 32 bits (high == 0), the ID is exactly 6 characters.
func (g *FeistelIDGenerator) Generate(sequence uint64) string {
	low := uint32(sequence) //nolint:gosec // intentional bit extraction
	high := uint32(sequence >> 32)

	lowStr := encodeBase58Padded(g.permuted(low))

	if high == 0 {
		return lowStr
	}

	return lowStr + encodeBase58(high)
}

func deriveFeistelKeys(seed uint32) [4]uint32 {
	// SplitMix32-style progression provides deterministic, well-dispersed key material.
	x := seed + splitMix32Increment

	var keys [4]uint32

	for i := range keys {
		x += splitMix32Increment
		z := x
		z ^= z >> 16
		z *= 0x85ebca6b
		z ^= z >> 13
		z *= 0xc2b2ae35
		z ^= z >> 16

		keys[i] = z
	}

	return keys
}

func feistelPermute(value uint32, keys [4]uint32) uint32 {
	left := (value >> 16) & feistelHalfMask
	right := value & feistelHalfMask

	for _, key := range keys {
		nextLeft := right
		nextRight := (left ^ feistelRound(right, key)) & feistelHalfMask

		left = nextLeft
		right = nextRight
	}

	return (left << 16) | right
}

func feistelRound(half, key uint32) uint32 {
	x := (half ^ key) * feistelRoundMul
	x ^= x >> 16

	return x & feistelHalfMask
}

// encodeBase58Padded encodes value as base58 left-padded with '1' to exactly lowPartLen chars.
func encodeBase58Padded(value uint32) string {
	var buffer [lowPartLen]byte

	// Fill with the Base58 zero character ('1').
	for i := range buffer {
		buffer[i] = Base58Alphabet[0]
	}

	index := lowPartLen

	for value > 0 {
		remainder := value % base58RadixSize
		value /= base58RadixSize
		index--

		buffer[index] = Base58Alphabet[int(remainder)]
	}

	return string(buffer[:])
}

// encodeBase58 encodes value as a variable-length base58 string (no padding).
func encodeBase58(value uint32) string {
	if value == 0 {
		return string(Base58Alphabet[0])
	}

	var buffer [6]byte

	index := len(buffer)

	for value > 0 {
		remainder := value % base58RadixSize
		value /= base58RadixSize
		index--

		buffer[index] = Base58Alphabet[int(remainder)]
	}

	return string(buffer[index:])
}
