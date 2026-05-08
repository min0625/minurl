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
)

// IDGenerator describes short ID generation operations required by ShortURLService.
type IDGenerator interface {
	Generate(sequence uint32) string
}

// FeistelIDGenerator produces base58 short IDs from a uint32 sequence.
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

// permuted returns the Feistel-permuted value of the given sequence.
func (g *FeistelIDGenerator) permuted(sequence uint32) uint32 {
	return feistelPermute(sequence, g.keys)
}

// Generate converts a monotonically increasing sequence into a base58 short ID.
func (g *FeistelIDGenerator) Generate(sequence uint32) string {
	permuted := g.permuted(sequence)

	return encodeBase58(permuted)
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
