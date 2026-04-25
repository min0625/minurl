// Copyright 2024 The MinURL Authors

// Package service implements the business logic for the MinURL service.
package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/min0625/minurl/internal/model"
)

// ShortURLService manages short URL resources using mock in-memory storage.
type ShortURLService struct {
	mu    sync.RWMutex
	store map[string]model.ShortURL
}

// NewShortURLService returns a new ShortURLService with pre-seeded mock data.
func NewShortURLService() *ShortURLService {
	s := &ShortURLService{
		store: make(map[string]model.ShortURL),
	}
	s.seed()

	return s
}

func (s *ShortURLService) seed() {
	mocks := []model.ShortURL{
		{
			ID:          "abc123",
			OriginalURL: "https://example.com",
			CreateTime:  time.Now().Add(-48 * time.Hour),
		},
		{
			ID:          "def456",
			OriginalURL: "https://github.com/min0625/minurl",
			CreateTime:  time.Now().Add(-24 * time.Hour),
		},
	}

	for i := range mocks {
		s.store[mocks[i].ID] = mocks[i]
	}
}

// Create creates a new short URL and returns it.
func (s *ShortURLService) Create(originalURL string) (*model.ShortURL, error) {
	for {
		id, err := generateID()
		if err != nil {
			return nil, fmt.Errorf("generate id: %w", err)
		}

		entry := model.ShortURL{
			ID:          id,
			OriginalURL: originalURL,
			CreateTime:  time.Now().UTC(),
		}

		s.mu.Lock()
		if _, exists := s.store[id]; !exists {
			s.store[id] = entry
			s.mu.Unlock()

			result := entry

			return &result, nil
		}
		s.mu.Unlock()
	}
}

// Get retrieves a short URL by ID.
func (s *ShortURLService) Get(id string) (*model.ShortURL, bool) {
	s.mu.RLock()
	entry, ok := s.store[id]
	s.mu.RUnlock()

	if !ok {
		return nil, false
	}

	result := entry

	return &result, true
}

func generateID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}
