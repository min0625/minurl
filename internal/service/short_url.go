// Copyright 2024 The MinURL Authors

// Package service implements the business logic for the MinURL service.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/min0625/minurl/internal/model"
)

// ShortURLService manages short URL resources using pluggable storage and counters.
type ShortURLService struct {
	store   ShortURLStorage
	counter ShortURLCounter
	idGen   IDGenerator
}

// NewShortURLServiceWithAllDependencies returns a new ShortURLService with custom storage,
// counter, and ID generator backends.
func NewShortURLServiceWithAllDependencies(
	store ShortURLStorage,
	counter ShortURLCounter,
	idGen IDGenerator,
) *ShortURLService {
	if store == nil {
		panic("short url storage must not be nil")
	}

	if counter == nil {
		panic("short url counter must not be nil")
	}

	if idGen == nil {
		idGen = NewDefaultFeistelIDGenerator()
	}

	s := &ShortURLService{
		store:   store,
		counter: counter,
		idGen:   idGen,
	}

	return s
}

// Create creates a new short URL and returns it.
func (s *ShortURLService) Create(ctx context.Context, originalURL string) (*model.ShortURL, error) {
	for {
		next, err := s.counter.Next(ctx)
		if err != nil {
			return nil, fmt.Errorf("next sequence: %w", err)
		}

		id := s.idGen.Generate(next)
		entry := model.ShortURL{
			ID:          id,
			OriginalURL: originalURL,
			CreateTime:  time.Now().UTC(),
		}

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
func (s *ShortURLService) Get(ctx context.Context, id string) (*model.ShortURL, bool, error) {
	entry, ok, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("get short url from store: %w", err)
	}

	if !ok {
		return nil, false, nil
	}

	result := entry

	return &result, true, nil
}
