// Copyright 2024 The MinURL Authors

package service_test

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/min0625/minurl/internal/service"
	"github.com/min0625/minurl/internal/testhelpers"
)

const customShortURLID = "custom123"

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
		service.ShortURL{ID: customShortURLID, OriginalURL: "https://example.org/custom"},
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if entry.ID != customShortURLID {
		t.Fatalf("Create() id = %q, want %q", entry.ID, customShortURLID)
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
		service.ShortURL{ID: customShortURLID, OriginalURL: "https://example.org/duplicate"},
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

	for i := range 2000 {
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

func TestShortURLServiceGetReturnsNotFoundWhenExpired(t *testing.T) {
	t.Parallel()

	svc := newTestShortURLService(t)

	past := time.Now().UTC().Add(-time.Hour)

	entry, err := svc.Create(
		context.Background(),
		service.ShortURL{
			OriginalURL: "https://example.org/expired",
			ExpireTime:  &past,
		},
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, ok, err := svc.Get(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if ok {
		t.Fatalf("Get() ok = true for expired URL, want false")
	}

	if got != nil {
		t.Fatalf("Get() returned non-nil entry for expired URL")
	}
}

func TestShortURLServiceGetReturnsFutureExpireURL(t *testing.T) {
	t.Parallel()

	svc := newTestShortURLService(t)

	future := time.Now().UTC().Add(time.Hour)

	entry, err := svc.Create(
		context.Background(),
		service.ShortURL{
			OriginalURL: "https://example.org/future",
			ExpireTime:  &future,
		},
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, ok, err := svc.Get(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if !ok {
		t.Fatalf("Get() ok = false for non-expired URL, want true")
	}

	if got.OriginalURL != "https://example.org/future" {
		t.Fatalf("Get() original_url = %q, want %q", got.OriginalURL, "https://example.org/future")
	}

	if got.ExpireTime == nil {
		t.Fatal("Get() ExpireTime = nil, want non-nil")
	}
}

func TestShortURLServiceGetNilExpireTimeIsPermanent(t *testing.T) {
	t.Parallel()

	svc := newTestShortURLService(t)

	entry, err := svc.Create(
		context.Background(),
		service.ShortURL{OriginalURL: "https://example.org/permanent"},
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, ok, err := svc.Get(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if !ok {
		t.Fatalf("Get() ok = false for permanent URL, want true")
	}

	if got.ExpireTime != nil {
		t.Fatalf("Get() ExpireTime = %v, want nil for permanent URL", got.ExpireTime)
	}
}

func TestIsValidOriginalURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{name: "http", rawURL: "http://example.com/path"},
		{name: "https", rawURL: "https://example.com/path?q=1#frag"},
		{name: "unicode host", rawURL: "https://例え.テスト/path"},
		{name: "host with port", rawURL: "http://example.com:8080/path"},
		{name: "empty", rawURL: "", wantErr: true},
		{name: "javascript", rawURL: "javascript:alert(1)", wantErr: true},
		{name: "data", rawURL: "data:text/html,<h1>hi</h1>", wantErr: true},
		{name: "file", rawURL: "file:///etc/passwd", wantErr: true},
		{name: "ftp", rawURL: "ftp://example.com/x", wantErr: true},
		{name: "protocol relative", rawURL: "//example.com", wantErr: true},
		{name: "relative path", rawURL: "/relative/path", wantErr: true},
		{name: "no host", rawURL: "http://", wantErr: true},
		{name: "port without host", rawURL: "http://:8080/", wantErr: true},
		{name: "space in host", rawURL: "http://exa mple.com", wantErr: true},
		{name: "userinfo", rawURL: "https://www.example.com@evil.example.org/", wantErr: true},
		{
			// url.Parse only screens control bytes before the "#", so a CRLF in the
			// fragment parses fine and would otherwise land in the Location header.
			name:    "crlf in fragment",
			rawURL:  "https://example.com/#\r\nSet-Cookie: pwned=1",
			wantErr: true,
		},
		{name: "control character in path", rawURL: "https://example.com/a\x01b", wantErr: true},
		{
			// Length is a storage limit enforced by the create schema, not a safety rule:
			// an over-long row that is already stored is still safe to redirect to.
			name:   "over max length",
			rawURL: "https://example.com/" + strings.Repeat("a", service.MaxOriginalURLLen),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := service.IsValidOriginalURL(tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("IsValidOriginalURL(%q) error = %v, wantErr = %v", tt.rawURL, err, tt.wantErr)
			}
		})
	}
}

// TestBase58AlphabetContents pins the alphabet itself. Everything else compares against it —
// the ID generator indexes it, and TestRegisterPublishesShortIDConstraints builds the expected
// request pattern from it — so a character added to or dropped from the constant would move
// both sides of those checks together and pass.
func TestBase58AlphabetContents(t *testing.T) {
	t.Parallel()

	// encodeBase58 divides by an unexported radix constant of 58 and indexes the
	// alphabet with the remainder; a shorter alphabet panics at runtime, not here.
	if len(service.Base58Alphabet) != 58 {
		t.Fatalf("len(Base58Alphabet) = %d, want 58", len(service.Base58Alphabet))
	}

	for _, ch := range "0OIl" {
		if strings.ContainsRune(service.Base58Alphabet, ch) {
			t.Errorf("Base58Alphabet contains confusable character %q", ch)
		}
	}
}

// TestGeneratedIDsSatisfyPublishedIDConstraints ties the ID generator to the constraints the
// {id} path params publish. Generate() maxes out at lowPartLen (6) padded characters plus a
// 6-character suffix, which is exactly MaxShortURLIDLen — nothing else links the two. An ID
// the generator can mint but the path schema rejects is a short URL that POST returns and
// GET /{id} can never resolve.
func TestGeneratedIDsSatisfyPublishedIDConstraints(t *testing.T) {
	t.Parallel()

	gen := service.NewDefaultFeistelIDGenerator()
	base58 := regexp.MustCompile("^[" + service.Base58Alphabet + "]+$")

	sequences := []uint64{
		0,
		1,
		math.MaxUint32,     // last ID with no suffix
		math.MaxUint32 + 1, // first ID with a suffix
		math.MaxUint64,     // longest possible ID
	}

	for _, seq := range sequences {
		id := gen.Generate(seq)

		if len(id) > service.MaxShortURLIDLen {
			t.Errorf(
				"Generate(%d) = %q, length %d exceeds MaxShortURLIDLen %d",
				seq, id, len(id), service.MaxShortURLIDLen,
			)
		}

		if !base58.MatchString(id) {
			t.Errorf("Generate(%d) = %q, want only Base58 characters", seq, id)
		}
	}
}
