// Copyright 2024 The MinURL Authors

// Package service implements the business logic for the MinURL service.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/min0625/minurl/internal/model"
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
// If entry.ID is provided, it will be used as the short URL identifier.
func (s *ShortURLService) Create(
	ctx context.Context,
	entry model.ShortURL,
) (*model.ShortURL, error) {
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

func validateShortURLID(id string) error {
	if id == "" {
		return errors.New("id is required")
	}

	if len(id) > 10 {
		return errors.New("id is too long")
	}

	for _, ch := range id {
		if !strings.ContainsRune(base58Alphabet, ch) {
			return errors.New("id contains invalid characters")
		}
	}

	return nil
}

// IsValidShortURLID returns nil when id conforms to allowed short URL identifier rules, otherwise returns an error.
func IsValidShortURLID(id string) error {
	return validateShortURLID(id)
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
