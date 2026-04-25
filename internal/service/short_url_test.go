// Copyright 2024 The MinURL Authors

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/min0625/minurl/internal/model"
	"github.com/min0625/minurl/internal/service"
)

func TestShortURLServiceCreateAndGet(t *testing.T) {
	t.Parallel()

	svc := service.NewShortURLService()

	entry, err := svc.Create(context.Background(), "https://example.org/path")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if entry.ID == "" {
		t.Fatal("Create() returned empty ID")
	}

	if entry.CreateTime.IsZero() {
		t.Fatal("Create() returned zero CreateTime")
	}

	got, ok := svc.Get(context.Background(), entry.ID)
	if !ok {
		t.Fatalf("Get(%q) returned ok = false", entry.ID)
	}

	if got.OriginalURL != "https://example.org/path" {
		t.Fatalf(
			"Get(%q) original_url = %q, want %q",
			entry.ID,
			got.OriginalURL,
			"https://example.org/path",
		)
	}

	if got.ID != entry.ID {
		t.Fatalf("Get(%q) id = %q, want %q", entry.ID, got.ID, entry.ID)
	}

	if _, ok := svc.Get(context.Background(), "missing"); ok {
		t.Fatal("Get(missing) returned ok = true, want false")
	}
}

func TestShortURLServiceGetReturnsCopy(t *testing.T) {
	t.Parallel()

	svc := service.NewShortURLService()

	entry, err := svc.Create(context.Background(), "https://example.org/original")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, ok := svc.Get(context.Background(), entry.ID)
	if !ok {
		t.Fatalf("Get(%q) returned ok = false", entry.ID)
	}

	got.OriginalURL = "https://example.org/mutated"

	gotAgain, ok := svc.Get(context.Background(), entry.ID)
	if !ok {
		t.Fatalf("Get(%q) second read returned ok = false", entry.ID)
	}

	if gotAgain.OriginalURL != "https://example.org/original" {
		t.Fatalf(
			"stored value mutated via returned pointer: got %q, want %q",
			gotAgain.OriginalURL,
			"https://example.org/original",
		)
	}
}

func TestShortURLServiceCreateGeneratesUniqueBase58IDs(t *testing.T) {
	t.Parallel()

	svc := service.NewShortURLService()
	seen := make(map[string]struct{}, 2000)

	for i := 0; i < 2000; i++ {
		entry, err := svc.Create(context.Background(), "https://example.org/batch")
		if err != nil {
			t.Fatalf("Create() error at iteration %d: %v", i, err)
		}

		if entry.ID == "" {
			t.Fatalf("Create() returned empty ID at iteration %d", i)
		}

		if !isBase58(entry.ID) {
			t.Fatalf("Create() returned non-base58 ID %q at iteration %d", entry.ID, i)
		}

		if _, exists := seen[entry.ID]; exists {
			t.Fatalf("Create() returned duplicate ID %q at iteration %d", entry.ID, i)
		}

		seen[entry.ID] = struct{}{}
	}
}

func TestShortURLServiceWithCustomStorage(t *testing.T) {
	t.Parallel()

	store := &testStorage{entries: make(map[string]model.ShortURL)}
	svc := service.NewShortURLServiceWithStorage(store)

	if len(store.entries) != 0 {
		t.Fatalf("custom storage should start empty, got %d entries", len(store.entries))
	}

	entry, err := svc.Create(context.Background(), "https://example.org/custom-store")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, ok := store.entries[entry.ID]; !ok {
		t.Fatalf("custom storage does not contain created id %q", entry.ID)
	}
}

func TestShortURLServiceCreateHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	svc := service.NewShortURLService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := svc.Create(ctx, "https://example.org/canceled"); err == nil {
		t.Fatal("Create() error = nil, want context canceled error")
	} else if !errors.Is(err, context.Canceled) {
		t.Fatalf("Create() error = %v, want context canceled", err)
	}
}

func isBase58(id string) bool {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

	for _, ch := range id {
		if !strings.ContainsRune(alphabet, ch) {
			return false
		}
	}

	return true
}

type testStorage struct {
	entries map[string]model.ShortURL
}

func (s *testStorage) CreateIfAbsent(_ context.Context, entry model.ShortURL) (bool, error) {
	if _, exists := s.entries[entry.ID]; exists {
		return false, nil
	}

	s.entries[entry.ID] = entry

	return true, nil
}

func (s *testStorage) GetByID(_ context.Context, id string) (model.ShortURL, bool, error) {
	entry, ok := s.entries[id]

	return entry, ok, nil
}

func (s *testStorage) Upsert(_ context.Context, entry model.ShortURL) error {
	s.entries[entry.ID] = entry

	return nil
}
