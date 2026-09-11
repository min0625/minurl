// Copyright 2024 The MinURL Authors

package service

import "context"

// ShortURLServicer is the contract for short URL business logic operations.
// Implementations must contain no HTTP or transport concerns: no status codes, no headers,
// no request or response types. ShortURL itself is shared with the handler rather than
// duplicated as a DTO, so the model does carry its wire schema; the methods below do not.
type ShortURLServicer interface {
	// Create creates a new short URL and returns it.
	// If entry.ID is provided, it will be used as the short URL identifier.
	// Returns ErrShortURLIDConflict if the given ID already exists.
	Create(ctx context.Context, entry ShortURL) (*ShortURL, error)

	// Get retrieves the short URL with the given ID.
	// Returns (nil, false, nil) when the ID is not found.
	Get(ctx context.Context, id string) (*ShortURL, bool, error)
}

// Compile-time assertion: ShortURLService must satisfy ShortURLServicer.
var _ ShortURLServicer = (*ShortURLService)(nil)
