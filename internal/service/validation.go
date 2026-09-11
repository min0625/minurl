// Copyright 2024 The MinURL Authors

package service

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
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
)

// IsValidOriginalURL returns nil when rawURL is an absolute http or https URL with a hostname,
// otherwise returns a descriptive error.
//
// Enforced in two places: OriginalURL.Resolve rejects bad input on create, and the redirect
// handler runs it again because it also serves rows written before this rule existed.
//
// It checks safety only, never length: there is deliberately no length limit. Storage
// limits belong to the store layer — see the CreateIfAbsent comment in mysql.go — and are
// not a reason to put a limit back here.
func IsValidOriginalURL(rawURL string) error {
	if rawURL == "" {
		return errors.New("original URL is required")
	}

	// isSpaceOrControl below sees runes, not bytes, and an invalid UTF-8 sequence decodes
	// to U+FFFD, which is neither a space nor a control character. A raw C1 byte would
	// therefore slip past while its correctly encoded form is rejected, and the raw bytes
	// are what gets stored and replayed into the Location header.
	//
	// A request never reaches here unsanitized: encoding/json already substitutes U+FFFD
	// for an invalid byte, so this screen bites on the redirect path, where rows written
	// by an older version or by another writer of the same database are re-validated.
	if !utf8.ValidString(rawURL) {
		return errors.New("original URL must be valid UTF-8")
	}

	// url.Parse applies its own control-byte guard only to the part before "#", so a
	// CR/LF in the fragment parses cleanly and then reaches the Location header verbatim.
	// Spaces are worse: url.Parse accepts them anywhere after the authority, so
	// "https://example.com/a b" round-trips into the Location header unencoded. The
	// invisible format characters are worse still, and for the same reason as userinfo
	// below: a U+202E in the path reverses the text after it, so a URL ending in
	// "gnp.exe" is displayed as one ending in "exe.png". None of them is legal in a
	// URI, and the raw string is what gets stored and served, so screen them all
	// before parsing.
	if strings.ContainsFunc(rawURL, isNotURLPrintable) {
		return errors.New("original URL must not contain whitespace, control or invisible characters")
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

// isNotURLPrintable reports whether r may not appear literally in a URL. !IsPrint covers the
// C0/C1 controls and DEL, the separators including the no-break space, and — unlike
// IsControl, which is category Cc only — the invisible format characters (U+200B, U+202E,
// U+FEFF, the bidi isolates). IsSpace adds the one blank IsPrint accepts, ASCII space.
func isNotURLPrintable(r rune) bool {
	return !unicode.IsPrint(r) || unicode.IsSpace(r)
}
