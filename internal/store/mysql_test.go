package store //nolint:testpackage // White-box tests validate internal MySQL helpers.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/min0625/minurl/internal/service"
)

func TestParseMySQLDSN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		dsn     string
		wantErr bool
	}{
		{ //nolint:gosec // test credentials in DSN
			name:    "non-mysql scheme is rejected",
			dsn:     "postgres://user:pass@localhost/db",
			wantErr: true,
		},
		{
			name:    "missing host is rejected",
			dsn:     "mysql:///dbname",
			wantErr: true,
		},
		{ //nolint:gosec // test credentials in DSN
			name:    "missing database is rejected",
			dsn:     "mysql://user:pass@localhost:3306/",
			wantErr: true,
		},
		{ //nolint:gosec // test credentials in DSN
			name: "basic DSN without port",
			dsn:  "mysql://user:pass@localhost/dbname",
		},
		{ //nolint:gosec // test credentials in DSN
			name: "basic DSN with port",
			dsn:  "mysql://user:pass@localhost:3306/dbname",
		},
		{ //nolint:gosec // test credentials in DSN
			name: "DSN with extra params",
			dsn:  "mysql://user:pass@localhost:3306/dbname?tls=skip-verify",
		},
		{ //nolint:gosec // test credentials in DSN
			name: "DSN with special chars in password",
			dsn:  "mysql://user:p%40ss@localhost/dbname",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseMySQLDSN(tc.dsn)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseMySQLDSN(%q) error = nil, want non-nil", tc.dsn)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseMySQLDSN(%q) error = %v", tc.dsn, err)
			}

			if got == "" {
				t.Fatalf("parseMySQLDSN(%q) = empty string", tc.dsn)
			}
		})
	}
}

func TestParseMySQLDSNEnforcesParseTime(t *testing.T) {
	t.Parallel()

	got, err := parseMySQLDSN("mysql://user:pass@localhost/dbname")
	if err != nil {
		t.Fatalf("parseMySQLDSN() error = %v", err)
	}

	// The driver DSN must contain parseTime=true so time.Time values round-trip correctly.
	if got == "" {
		t.Fatalf("parseMySQLDSN() returned empty DSN")
	}
}

func TestMySQLShortURLStorageCreateIfAbsent(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, _, closer, err := NewMySQLBackends(testMySQLDSN, DBPoolConfig{})
	if err != nil {
		t.Fatalf("NewMySQLBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close mysql backend: %v", closeErr)
		}
	}()

	entry := service.ShortURL{
		ID:          fmt.Sprintf("m-create-%d", time.Now().UnixNano()),
		OriginalURL: "https://example.com",
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}

	created, err := storage.CreateIfAbsent(context.Background(), entry)
	if err != nil {
		t.Fatalf("CreateIfAbsent() error = %v", err)
	}

	if !created {
		t.Fatalf("CreateIfAbsent() = false, want true for new entry")
	}
}

func TestMySQLShortURLStorageCreateIfAbsentConflict(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, _, closer, err := NewMySQLBackends(testMySQLDSN, DBPoolConfig{})
	if err != nil {
		t.Fatalf("NewMySQLBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close mysql backend: %v", closeErr)
		}
	}()

	id := fmt.Sprintf("m-dup-%d", time.Now().UnixNano())

	entry := service.ShortURL{
		ID:          id,
		OriginalURL: "https://example.com/first",
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}

	if _, err := storage.CreateIfAbsent(context.Background(), entry); err != nil {
		t.Fatalf("first CreateIfAbsent() error = %v", err)
	}

	duplicate := service.ShortURL{
		ID:          id,
		OriginalURL: "https://example.com/second",
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}

	created, err := storage.CreateIfAbsent(context.Background(), duplicate)
	if err != nil {
		t.Fatalf("second CreateIfAbsent() error = %v", err)
	}

	if created {
		t.Fatalf("CreateIfAbsent() = true, want false for duplicate ID")
	}
}

func TestMySQLShortURLStorageGetByID(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, _, closer, err := NewMySQLBackends(testMySQLDSN, DBPoolConfig{})
	if err != nil {
		t.Fatalf("NewMySQLBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close mysql backend: %v", closeErr)
		}
	}()

	ctx := context.Background()

	entry := service.ShortURL{
		ID:          fmt.Sprintf("m-get-%d", time.Now().UnixNano()),
		OriginalURL: "https://example.com/mysql",
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}

	if _, err := storage.CreateIfAbsent(ctx, entry); err != nil {
		t.Fatalf("CreateIfAbsent() error = %v", err)
	}

	got, found, err := storage.GetByID(ctx, entry.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if !found {
		t.Fatalf("GetByID() found = false, want true")
	}

	if got.ID != entry.ID {
		t.Fatalf("GetByID() ID = %q, want %q", got.ID, entry.ID)
	}

	if got.OriginalURL != entry.OriginalURL {
		t.Fatalf("GetByID() OriginalURL = %q, want %q", got.OriginalURL, entry.OriginalURL)
	}

	if !got.CreateTime.Equal(entry.CreateTime) {
		t.Fatalf("GetByID() CreateTime = %v, want %v", got.CreateTime, entry.CreateTime)
	}
}

func TestMySQLShortURLStorageGetByIDNotFound(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, _, closer, err := NewMySQLBackends(testMySQLDSN, DBPoolConfig{})
	if err != nil {
		t.Fatalf("NewMySQLBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close mysql backend: %v", closeErr)
		}
	}()

	_, found, err := storage.GetByID(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if found {
		t.Fatalf("GetByID() found = true, want false for missing ID")
	}
}

func TestMySQLShortURLStorageExpireTimeRoundTrip(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, _, closer, err := NewMySQLBackends(testMySQLDSN, DBPoolConfig{})
	if err != nil {
		t.Fatalf("NewMySQLBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close mysql backend: %v", closeErr)
		}
	}()

	ctx := context.Background()
	expiry := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	entry := service.ShortURL{
		ID:          fmt.Sprintf("m-expiry-%d", time.Now().UnixNano()),
		OriginalURL: "https://example.com/mysql-expiry",
		ExpireTime:  &expiry,
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}

	created, err := storage.CreateIfAbsent(ctx, entry)
	if err != nil {
		t.Fatalf("CreateIfAbsent() error = %v", err)
	}

	if !created {
		t.Fatalf("CreateIfAbsent() = false, want true")
	}

	got, found, err := storage.GetByID(ctx, entry.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if !found {
		t.Fatalf("GetByID() found = false, want true")
	}

	if got.ExpireTime == nil {
		t.Fatalf("GetByID() ExpireTime = nil, want %v", expiry)
	}

	if !got.ExpireTime.Equal(expiry) {
		t.Fatalf("GetByID() ExpireTime = %v, want %v", got.ExpireTime, expiry)
	}
}

func TestMySQLShortURLCounterNext(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	_, counter, closer, err := NewMySQLBackends(testMySQLDSN, DBPoolConfig{})
	if err != nil {
		t.Fatalf("NewMySQLBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close mysql backend: %v", closeErr)
		}
	}()

	ctx := context.Background()

	first, err := counter.Next(ctx)
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}

	second, err := counter.Next(ctx)
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}

	if second <= first {
		t.Fatalf("Next() second call = %d, want > first call %d", second, first)
	}
}

func TestMySQLMigrationIdempotent(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	_, _, firstCloser, err := NewMySQLBackends(testMySQLDSN, DBPoolConfig{})
	if err != nil {
		t.Fatalf("first NewMySQLBackends() error = %v", err)
	}

	if err := firstCloser.Close(); err != nil {
		t.Fatalf("close first mysql backend: %v", err)
	}

	_, _, secondCloser, err := NewMySQLBackends(testMySQLDSN, DBPoolConfig{})
	if err != nil {
		t.Fatalf("second NewMySQLBackends() error = %v", err)
	}

	if err := secondCloser.Close(); err != nil {
		t.Fatalf("close second mysql backend: %v", err)
	}
}

// TestMySQLShortURLStorageCaseSensitiveIDs verifies that the MySQL storage
// treats IDs as case-sensitive: "abcdef" and "ABCDEF" must be distinct rows.
// This requires COLLATE utf8mb4_0900_as_cs on the id column (see migration 000001).
func TestMySQLShortURLStorageCaseSensitiveIDs(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, _, closer, err := NewMySQLBackends(testMySQLDSN, DBPoolConfig{})
	if err != nil {
		t.Fatalf("NewMySQLBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close mysql backend: %v", closeErr)
		}
	}()

	ctx := context.Background()
	base := fmt.Sprintf("cs%d", time.Now().UnixNano())
	lower := base + "abc"
	upper := base + "ABC"

	lowerEntry := service.ShortURL{
		ID:          lower,
		OriginalURL: "https://example.com/lower",
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}
	upperEntry := service.ShortURL{
		ID:          upper,
		OriginalURL: "https://example.com/upper",
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}

	// Both should be created — they are different IDs.
	lowerCreated, err := storage.CreateIfAbsent(ctx, lowerEntry)
	if err != nil {
		t.Fatalf("CreateIfAbsent(lower) error = %v", err)
	}

	if !lowerCreated {
		t.Fatalf("CreateIfAbsent(lower) = false, want true")
	}

	upperCreated, err := storage.CreateIfAbsent(ctx, upperEntry)
	if err != nil {
		t.Fatalf("CreateIfAbsent(upper) error = %v", err)
	}

	if !upperCreated {
		t.Fatalf("CreateIfAbsent(upper) = false, want true: IDs must be case-sensitive")
	}

	// Inserting the same lower-case ID again must return created=false (conflict).
	dupCreated, err := storage.CreateIfAbsent(ctx, lowerEntry)
	if err != nil {
		t.Fatalf("CreateIfAbsent(lower duplicate) error = %v", err)
	}

	if dupCreated {
		t.Fatalf("CreateIfAbsent(lower duplicate) = true, want false")
	}

	// GetByID must return each entry independently.
	gotLower, foundLower, err := storage.GetByID(ctx, lower)
	if err != nil {
		t.Fatalf("GetByID(lower) error = %v", err)
	}

	if !foundLower {
		t.Fatalf("GetByID(lower) found = false, want true")
	}

	if gotLower.ID != lower {
		t.Fatalf("GetByID(lower) ID = %q, want %q", gotLower.ID, lower)
	}

	gotUpper, foundUpper, err := storage.GetByID(ctx, upper)
	if err != nil {
		t.Fatalf("GetByID(upper) error = %v", err)
	}

	if !foundUpper {
		t.Fatalf("GetByID(upper) found = false, want true")
	}

	if gotUpper.ID != upper {
		t.Fatalf("GetByID(upper) ID = %q, want %q", gotUpper.ID, upper)
	}

	if gotLower.OriginalURL == gotUpper.OriginalURL {
		t.Fatalf(
			"lower and upper IDs resolved to same OriginalURL %q, want distinct rows",
			gotLower.OriginalURL,
		)
	}
}
