// Copyright 2024 The MinURL Authors

// Package service implements the business logic for the MinURL service.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ShortURLService manages short URL resources using pluggable storage and counters.
type ShortURLService struct {
	store   ShortURLStorage
	counter ShortURLCounter
	idGen   IDGenerator
}

// ErrShortURLIDConflict is returned when a provided short URL ID already exists.
var ErrShortURLIDConflict = errors.New("short url id already exists")

// NewShortURLServiceWithAllDependencies returns a new ShortURLService with custom storage,
// counter, and ID generator backends.
func NewShortURLServiceWithAllDependencies(
	store ShortURLStorage,
	counter ShortURLCounter,
	idGen IDGenerator,
) (*ShortURLService, error) {
	if store == nil {
		return nil, errors.New("short url storage must not be nil")
	}

	if counter == nil {
		return nil, errors.New("short url counter must not be nil")
	}

	if idGen == nil {
		idGen = NewDefaultFeistelIDGenerator()
	}

	s := &ShortURLService{
		store:   store,
		counter: counter,
		idGen:   idGen,
	}

	return s, nil
}

// Create creates a new short URL and returns it.
// If entry.ID is provided, it will be used as the short URL identifier.
// Otherwise, it generates a unique ID by querying an incrementing counter.
// On ID collision (rare in practice due to the counter design), the method
// retries by fetching the next counter value and regenerating an ID.
// This loop will eventually succeed unless the counter encounters an error.
func (s *ShortURLService) Create(
	ctx context.Context,
	entry ShortURL,
) (*ShortURL, error) {
	if entry.ID != "" {
		entry.CreateTime = time.Now().UTC()

		created, err := s.store.CreateIfAbsent(ctx, entry)
		if err != nil {
			return nil, fmt.Errorf("create short url in store: %w", err)
		}

		if !created {
			return nil, ErrShortURLIDConflict
		}

		result := entry

		return &result, nil
	}

	for {
		next, err := s.counter.Next(ctx)
		if err != nil {
			return nil, fmt.Errorf("next sequence: %w", err)
		}

		id := s.idGen.Generate(next)
		entry.ID = id
		entry.CreateTime = time.Now().UTC()

		created, err := s.store.CreateIfAbsent(ctx, entry)
		if err != nil {
			return nil, fmt.Errorf("create short url in store: %w", err)
		}

		if created {
			result := entry

			return &result, nil
		}
	}
}

// Get retrieves a short URL by ID.
// Returns (nil, false, nil) when the ID is not found or the URL has expired.
func (s *ShortURLService) Get(ctx context.Context, id string) (*ShortURL, bool, error) {
	entry, ok, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("get short url from store: %w", err)
	}

	if !ok {
		return nil, false, nil
	}

	if entry.ExpireTime != nil && time.Now().After(*entry.ExpireTime) {
		return nil, false, nil
	}

	result := entry

	return &result, true, nil
}
