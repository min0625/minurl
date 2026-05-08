// Copyright 2024 The MinURL Authors

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/min0625/minurl/internal/service"
	"github.com/min0625/minurl/internal/testhelpers"
)

func TestShortURLServiceCreateAndGet(t *testing.T) {
	t.Parallel()

	svc := newTestShortURLService(t)

	entry, err := svc.Create(
		context.Background(),
		service.ShortURL{OriginalURL: "https://example.org/path"},
	)
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

func TestShortURLServiceCreateWithCustomID(t *testing.T) {
	t.Parallel()

	svc := newTestShortURLService(t)

	entry, err := svc.Create(
		context.Background(),
		service.ShortURL{ID: "custom123", OriginalURL: "https://example.org/custom"},
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if entry.ID != "custom123" {
		t.Fatalf("Create() id = %q, want %q", entry.ID, "custom123")
	}

	if entry.OriginalURL != "https://example.org/custom" {
		t.Fatalf(
			"Create() original_url = %q, want %q",
			entry.OriginalURL,
			"https://example.org/custom",
		)
	}

	if _, err := svc.Create(
		context.Background(),
		service.ShortURL{ID: "custom123", OriginalURL: "https://example.org/duplicate"},
	); err == nil {
		t.Fatal("Create() error = nil, want ErrShortURLIDConflict")
	} else if !errors.Is(err, service.ErrShortURLIDConflict) {
		t.Fatalf("Create() error = %v, want %v", err, service.ErrShortURLIDConflict)
	}
}

func TestShortURLServiceGetReturnsCopy(t *testing.T) {
	t.Parallel()

	svc := newTestShortURLService(t)

	entry, err := svc.Create(
		context.Background(),
		service.ShortURL{OriginalURL: "https://example.org/original"},
	)
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

	svc := newTestShortURLService(t)
	seen := make(map[string]struct{}, 2000)

	for i := 0; i < 2000; i++ {
		entry, err := svc.Create(
			context.Background(),
			service.ShortURL{OriginalURL: "https://example.org/batch"},
		)
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

func TestIsValidShortURLID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid base58", "123456789A", false},
		{"empty", "", true},
		{"leading space", " abc", true},
		{"invalid character", "abcd!", true},
		{"disallowed base58 char 0", "0", true},
		{"disallowed base58 char O", "O", true},
		{"disallowed base58 char I", "I", true},
		{"disallowed base58 char l", "l", true},
		{"too long", "123456789ABCD", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.IsValidShortURLID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("IsValidShortURLID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestShortURLServiceWithCustomStorage(t *testing.T) {
	t.Parallel()

	store := testhelpers.NewStorage()

	svc, err := service.NewShortURLServiceWithAllDependencies(store, testhelpers.NewCounter(), nil)
	if err != nil {
		t.Fatalf("NewShortURLServiceWithAllDependencies() error = %v", err)
	}

	if len(store.GetEntries()) != 0 {
		t.Fatalf("custom storage should start empty, got %d entries", len(store.GetEntries()))
	}

	entry, err := svc.Create(
		context.Background(),
		service.ShortURL{OriginalURL: "https://example.org/custom-store"},
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, ok := store.GetEntries()[entry.ID]; !ok {
		t.Fatalf("custom storage does not contain created id %q", entry.ID)
	}
}

func TestShortURLServiceCreateHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	svc := newTestShortURLService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := svc.Create(
		ctx,
		service.ShortURL{OriginalURL: "https://example.org/canceled"},
	); err == nil {
		t.Fatal("Create() error = nil, want context canceled error")
	} else if !errors.Is(err, context.Canceled) {
		t.Fatalf("Create() error = %v, want context canceled", err)
	}
}

func TestShortURLServiceGetReturnsErrorWhenStorageFails(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("storage unavailable")
	store := testhelpers.NewStorage().WithGetError(expectedErr)

	svc, err := service.NewShortURLServiceWithAllDependencies(store, testhelpers.NewCounter(), nil)
	if err != nil {
		t.Fatalf("NewShortURLServiceWithAllDependencies() error = %v", err)
	}

	_, ok, err := svc.Get(context.Background(), "any")
	if err == nil {
		t.Fatal("Get() error = nil, want non-nil")
	}

	if !errors.Is(err, expectedErr) {
		t.Fatalf("Get() error = %v, want wrapped %v", err, expectedErr)
	}

	if ok {
		t.Fatal("Get() ok = true, want false")
	}
}

func TestShortURLServiceUsesInjectedIDGenerator(t *testing.T) {
	t.Parallel()

	store := testhelpers.NewStorage()
	idGen := testhelpers.NewIDGenerator("custom-id")

	svc, err := service.NewShortURLServiceWithAllDependencies(
		store,
		testhelpers.NewCounter(),
		idGen,
	)
	if err != nil {
		t.Fatalf("NewShortURLServiceWithAllDependencies() error = %v", err)
	}

	entry, err := svc.Create(
		context.Background(),
		service.ShortURL{OriginalURL: "https://example.org/injected"},
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if entry.ID != "custom-id" {
		t.Fatalf("Create() id = %q, want %q", entry.ID, "custom-id")
	}

	if idGen.CallCount() != 1 {
		t.Fatalf("ID generator calls = %d, want 1", idGen.CallCount())
	}
}

func TestFeistelIDGeneratorWithSeedIsDeterministic(t *testing.T) {
	t.Parallel()

	a := service.NewFeistelIDGeneratorWithSeed(12345)
	b := service.NewFeistelIDGeneratorWithSeed(12345)
	c := service.NewFeistelIDGeneratorWithSeed(54321)

	seq := []uint64{1, 2, 3, 1024, 65535}

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

	seq := []uint64{1, 2, 3, 1024, 65535}

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

func TestNewShortURLServiceWithAllDependenciesRejectsNilStorage(t *testing.T) {
	t.Parallel()

	_, err := service.NewShortURLServiceWithAllDependencies(nil, testhelpers.NewCounter(), nil)
	if err == nil {
		t.Fatal("NewShortURLServiceWithAllDependencies() error = nil, want non-nil")
	}
}

func TestNewShortURLServiceWithAllDependenciesRejectsNilCounter(t *testing.T) {
	t.Parallel()

	_, err := service.NewShortURLServiceWithAllDependencies(testhelpers.NewStorage(), nil, nil)
	if err == nil {
		t.Fatal("NewShortURLServiceWithAllDependencies() error = nil, want non-nil")
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

func newTestShortURLService(t *testing.T) *service.ShortURLService {
	t.Helper()

	store := testhelpers.NewStorage()
	counter := testhelpers.NewCounter()

	svc, err := service.NewShortURLServiceWithAllDependencies(store, counter, nil)
	if err != nil {
		t.Fatalf("NewShortURLServiceWithAllDependencies() error = %v", err)
	}

	return svc
}
