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
}

// NewShortURLService returns a new ShortURLService.
func NewShortURLService() *ShortURLService {
	return NewShortURLServiceWithStorage(NewInMemoryShortURLStorage())
}

// NewShortURLServiceWithStorage returns a new ShortURLService with a custom storage backend.
func NewShortURLServiceWithStorage(store ShortURLStorage) *ShortURLService {
	return NewShortURLServiceWithDependencies(store, NewInMemoryShortURLCounter())
}

// NewShortURLServiceWithDependencies returns a new ShortURLService with custom storage and counter backends.
func NewShortURLServiceWithDependencies(
	store ShortURLStorage,
	counter ShortURLCounter,
) *ShortURLService {
	if store == nil {
		store = NewInMemoryShortURLStorage()
	}

	if counter == nil {
		counter = NewInMemoryShortURLCounter()
	}

	s := &ShortURLService{
		store:   store,
		counter: counter,
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

		id := generateID(next)
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

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var feistelKeys = [4]uint32{0xA5A5A5A5, 0x3C6EF372, 0x9E3779B9, 0x1BF5A8D3}

func generateID(sequence uint32) string {
	permuted := feistelPermute(sequence)

	return encodeBase58(permuted)
}

func feistelPermute(value uint32) uint32 {
	left := (value >> 16) & 0xFFFF
	right := value & 0xFFFF

	for _, key := range feistelKeys {
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
