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

	got, ok, err := svc.Get(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Get(%q) error = %v", entry.ID, err)
	}

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

	if _, ok, err := svc.Get(context.Background(), "missing"); err != nil {
		t.Fatalf("Get(missing) error = %v", err)
	} else if ok {
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

	got, ok, err := svc.Get(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Get(%q) error = %v", entry.ID, err)
	}

	if !ok {
		t.Fatalf("Get(%q) returned ok = false", entry.ID)
	}

	got.OriginalURL = "https://example.org/mutated"

	gotAgain, ok, err := svc.Get(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Get(%q) second read error = %v", entry.ID, err)
	}

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

func TestShortURLServiceGetReturnsErrorWhenStorageFails(t *testing.T) {
	t.Parallel()

	store := &testStorage{
		entries: make(map[string]model.ShortURL),
		getErr:  errors.New("storage unavailable"),
	}
	svc := service.NewShortURLServiceWithStorage(store)

	_, ok, err := svc.Get(context.Background(), "any")
	if err == nil {
		t.Fatal("Get() error = nil, want non-nil")
	}

	if !errors.Is(err, store.getErr) {
		t.Fatalf("Get() error = %v, want wrapped %v", err, store.getErr)
	}

	if ok {
		t.Fatal("Get() ok = true, want false")
	}
}

func TestShortURLServiceUsesInjectedIDGenerator(t *testing.T) {
	t.Parallel()

	store := &testStorage{entries: make(map[string]model.ShortURL)}
	idGen := &fixedIDGenerator{id: "custom-id"}

	svc := service.NewShortURLServiceWithAllDependencies(store, nil, idGen)

	entry, err := svc.Create(context.Background(), "https://example.org/injected")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if entry.ID != "custom-id" {
		t.Fatalf("Create() id = %q, want %q", entry.ID, "custom-id")
	}

	if idGen.calls != 1 {
		t.Fatalf("ID generator calls = %d, want 1", idGen.calls)
	}
}

func TestFeistelIDGeneratorWithSeedIsDeterministic(t *testing.T) {
	t.Parallel()

	a := service.NewFeistelIDGeneratorWithSeed(12345)
	b := service.NewFeistelIDGeneratorWithSeed(12345)
	c := service.NewFeistelIDGeneratorWithSeed(54321)

	seq := []uint32{1, 2, 3, 1024, 65535}

	for _, v := range seq {
		if gotA, gotB := a.Generate(v), b.Generate(v); gotA != gotB {
			t.Fatalf("same seed generated different IDs for seq %d: %q != %q", v, gotA, gotB)
		}

		if gotA, gotC := a.Generate(v), c.Generate(v); gotA == gotC {
			t.Fatalf("different seeds generated same ID for seq %d: %q", v, gotA)
		}
	}
}

func TestDefaultFeistelIDGeneratorUsesDefaultSeed(t *testing.T) {
	t.Parallel()

	const expectedDefaultSeed uint32 = 0xC0FFEE42

	defaultGen := service.NewDefaultFeistelIDGenerator()
	seedGen := service.NewFeistelIDGeneratorWithSeed(expectedDefaultSeed)

	seq := []uint32{1, 2, 3, 1024, 65535}

	for _, v := range seq {
		if gotDefault, gotSeed := defaultGen.Generate(
			v,
		), seedGen.Generate(
			v,
		); gotDefault != gotSeed {
			t.Fatalf(
				"default generator differs from default seed for seq %d: %q != %q",
				v,
				gotDefault,
				gotSeed,
			)
		}
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
	getErr  error
}

type fixedIDGenerator struct {
	id    string
	calls int
}

func (g *fixedIDGenerator) Generate(_ uint32) string {
	g.calls++

	return g.id
}

func (s *testStorage) CreateIfAbsent(_ context.Context, entry model.ShortURL) (bool, error) {
	if _, exists := s.entries[entry.ID]; exists {
		return false, nil
	}

	s.entries[entry.ID] = entry

	return true, nil
}

func (s *testStorage) GetByID(_ context.Context, id string) (model.ShortURL, bool, error) {
	if s.getErr != nil {
		return model.ShortURL{}, false, s.getErr
	}

	entry, ok := s.entries[id]

	return entry, ok, nil
}
