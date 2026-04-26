// Copyright 2024 The MinURL Authors

package service

const defaultFeistelSeed uint32 = 0xC0FFEE42

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

// Generate converts a monotonically increasing sequence into a base58 short ID.
func (g *FeistelIDGenerator) Generate(sequence uint32) string {
	permuted := feistelPermute(sequence, g.keys)

	return encodeBase58(permuted)
}

func deriveFeistelKeys(seed uint32) [4]uint32 {
	// SplitMix32-style progression provides deterministic, well-dispersed key material.
	x := seed + 0x9E3779B9

	var keys [4]uint32

	for i := range keys {
		x += 0x9E3779B9
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
	left := (value >> 16) & 0xFFFF
	right := value & 0xFFFF

	for _, key := range keys {
		nextLeft := right
		nextRight := (left ^ feistelRound(right, key)) & 0xFFFF

		left = nextLeft
		right = nextRight
	}

	return (left << 16) | right
}

func feistelRound(half, key uint32) uint32 {
	x := (half ^ key) * 0x45d9f3b
	x ^= x >> 16

	return x & 0xFFFF
}

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func encodeBase58(value uint32) string {
	if value == 0 {
		return string(base58Alphabet[0])
	}

	var buffer [6]byte

	index := len(buffer)

	for value > 0 {
		remainder := value % 58
		value /= 58
		index--

		buffer[index] = base58Alphabet[int(remainder)]
	}

	return string(buffer[index:])
}
