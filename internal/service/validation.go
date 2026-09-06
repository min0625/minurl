// Copyright 2024 The MinURL Authors

package service

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

const (
	// Base58Alphabet is the standard Base58 alphabet (no 0, O, I, l to avoid confusion).
	// This defines which characters are valid in a short URL identifier.
	Base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

	// MaxShortURLIDLen is the maximum allowed length for a short URL identifier.
	// Identifiers are validated by the request schema alone: the maxLength and pattern
	// tags on ShortURL.ID and the two {id} path params repeat this constant and
	// Base58Alphabet because struct tags cannot reference constants;
	// TestRegisterPublishesShortIDConstraints fails if any of them drift.
	MaxShortURLIDLen = 12

	// MaxOriginalURLLen is the maximum allowed length for an original URL, in characters.
	// Stored as TEXT, which tops out at 65535 bytes on MySQL; rejecting on the way in
	// turns an over-long URL into a 422 instead of a write failure.
	// 8192 leaves room for legitimately long URLs (OAuth callbacks, signed CDN links)
	// while staying under the storage limit even at 4 bytes per character.
	// This is a storage limit, not a safety rule, so it is enforced by OriginalURL.Schema
	// and not by IsValidOriginalURL, which also guards rows that are already stored.
	// The schema reads this constant directly, so unlike a maxLength struct tag it
	// cannot drift from it.
	MaxOriginalURLLen = 8192
)

// IsValidOriginalURL returns nil when rawURL is an absolute http or https URL with a hostname,
// otherwise returns a descriptive error.
//
// Enforced in two places: OriginalURL.Resolve rejects bad input on create, and the redirect
// handler runs it again because it also serves rows written before this rule existed.
// It checks safety only, never length: a stored row longer than MaxOriginalURLLen is still
// safe to redirect to, so length belongs on the create schema instead.
func IsValidOriginalURL(rawURL string) error {
	if rawURL == "" {
		return errors.New("original URL is required")
	}

	// url.Parse applies its own control-byte guard only to the part before "#", so a
	// CR/LF in the fragment parses cleanly and then reaches the Location header verbatim.
	// The raw string is what gets stored and served, so screen it before parsing.
	if strings.ContainsFunc(rawURL, unicode.IsControl) {
		return errors.New("original URL must not contain control characters")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse original URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("original URL must use the http or https scheme")
	}

	// Host keeps the port, so "http://:8080/" would pass a Host != "" check.
	if u.Hostname() == "" {
		return errors.New("original URL must have a host")
	}

	// "https://www.paypal.com@evil.example.com/" reads as paypal.com wherever the
	// Location header is previewed, so keep userinfo out of stored URLs.
	if u.User != nil {
		return errors.New("original URL must not contain userinfo")
	}

	return nil
}
