// Copyright 2024 The MinURL Authors

package testhelpers

import (
	"context"

	"github.com/min0625/minurl/internal/service"
)

// Storage is a test implementation of the ShortURLStorage interface.
// It stores entries in memory and supports configurable error injection.
type Storage struct {
	entries map[string]service.ShortURL
	getErr  error
}

// NewStorage creates a new in-memory test storage.
func NewStorage() *Storage {
	return &Storage{
		entries: make(map[string]service.ShortURL),
	}
}

// WithGetError configures the storage to return the specified error on GetByID calls.
func (s *Storage) WithGetError(err error) *Storage {
	s.getErr = err
	return s
}

// CreateIfAbsent creates an entry if the ID is not already present.
func (s *Storage) CreateIfAbsent(ctx context.Context, entry service.ShortURL) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if _, exists := s.entries[entry.ID]; exists {
		return false, nil
	}

	s.entries[entry.ID] = entry

	return true, nil
}

// GetByID retrieves an entry by ID. Returns the configured error if set.
func (s *Storage) GetByID(ctx context.Context, id string) (service.ShortURL, bool, error) {
	if s.getErr != nil {
		return service.ShortURL{}, false, s.getErr
	}

	if err := ctx.Err(); err != nil {
		return service.ShortURL{}, false, err
	}

	entry, ok := s.entries[id]

	return entry, ok, nil
}

// GetEntries returns all stored entries (for test verification).
func (s *Storage) GetEntries() map[string]service.ShortURL {
	return s.entries
}
