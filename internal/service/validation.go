// Copyright 2024 The MinURL Authors

package service

import (
	"errors"
	"strings"
)

const (
	// Base58Alphabet is the standard Base58 alphabet (no 0, O, I, l to avoid confusion).
	// This defines which characters are valid in a short URL identifier.
	Base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

	// MaxShortURLIDLen is the maximum allowed length for a short URL identifier.
	MaxShortURLIDLen = 10
)

// IsValidShortURLID returns nil when id conforms to allowed short URL identifier rules,
// otherwise returns a descriptive error.
func IsValidShortURLID(id string) error {
	if id == "" {
		return errors.New("id is required")
	}

	if len(id) > MaxShortURLIDLen {
		return errors.New("id is too long")
	}

	for _, ch := range id {
		if !strings.ContainsRune(Base58Alphabet, ch) {
			return errors.New("id contains invalid characters")
		}
	}

	return nil
}
