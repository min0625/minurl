// Copyright 2024 The MinURL Authors

package service

import (
	"context"
	"sync"

	"github.com/min0625/minurl/internal/model"
)

// ShortURLStorage describes storage operations required by ShortURLService.
type ShortURLStorage interface {
	CreateIfAbsent(ctx context.Context, entry model.ShortURL) (bool, error)
	GetByID(ctx context.Context, id string) (model.ShortURL, bool, error)
}

// InMemoryShortURLStorage is the default in-process storage implementation.
type InMemoryShortURLStorage struct {
	mu    sync.RWMutex
	store map[string]model.ShortURL
}

// NewInMemoryShortURLStorage creates an in-memory ShortURL storage backend.
func NewInMemoryShortURLStorage() *InMemoryShortURLStorage {
	return &InMemoryShortURLStorage{store: make(map[string]model.ShortURL)}
}

// CreateIfAbsent stores the entry if the ID does not already exist.
func (s *InMemoryShortURLStorage) CreateIfAbsent(
	_ context.Context,
	entry model.ShortURL,
) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.store[entry.ID]; exists {
		return false, nil
	}

	s.store[entry.ID] = entry

	return true, nil
}

// GetByID returns a short URL by its ID.
func (s *InMemoryShortURLStorage) GetByID(
	_ context.Context,
	id string,
) (model.ShortURL, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.store[id]

	return entry, ok, nil
}
