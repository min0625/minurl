package store //nolint:testpackage // White-box tests validate internal SQLite helpers.

import (
	"context"
	"testing"
	"time"

	"github.com/min0625/minurl/internal/service"
)

const (
	sqliteLowerURL = "https://example.com/lower"
	sqliteUpperURL = "https://example.com/upper"
)

func TestParseSQLiteDSN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		dsn     string
		want    string
		wantErr bool
	}{
		{
			name:    "bare file path is rejected",
			dsn:     "minurl.sqlite3",
			wantErr: true,
		},
		{
			name: "simple sqlite3 scheme",
			dsn:  "sqlite3://minurl.sqlite3",
			want: "minurl.sqlite3",
		},
		{
			name: "relative subdirectory",
			dsn:  "sqlite3://var/data/minurl.sqlite3",
			want: "var/data/minurl.sqlite3",
		},
		{
			name: "absolute path (three slashes)",
			dsn:  "sqlite3:///absolute/path/minurl.sqlite3",
			want: "/absolute/path/minurl.sqlite3",
		},
		{
			name: "with query params",
			dsn:  "sqlite3://minurl.sqlite3?cache=shared",
			want: "file:minurl.sqlite3?cache=shared",
		},
		{
			name:    "empty path",
			dsn:     "sqlite3://",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseSQLiteDSN(tc.dsn)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseSQLiteDSN(%q) error = nil, want non-nil", tc.dsn)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseSQLiteDSN(%q) error = %v", tc.dsn, err)
			}

			if got != tc.want {
				t.Fatalf("parseSQLiteDSN(%q) = %q, want %q", tc.dsn, got, tc.want)
			}
		})
	}
}

func TestOpenSQLiteDBUsesSingleConnectionPool(t *testing.T) {
	t.Parallel()

	db, err := openSQLiteDB("sqlite3:///" + t.TempDir() + "/pool.sqlite3")
	if err != nil {
		t.Fatalf("openSQLiteDB() error = %v", err)
	}

	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Fatalf("close db: %v", closeErr)
		}
	}()

	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}
}

func TestSQLiteMigrationIdempotent(t *testing.T) {
	t.Parallel()

	dsn := "sqlite3:///" + t.TempDir() + "/idempotent.sqlite3"

	_, _, firstCloser, err := NewSQLiteBackends(dsn)
	if err != nil {
		t.Fatalf("first NewSQLiteBackends() error = %v", err)
	}

	if err := firstCloser.Close(); err != nil {
		t.Fatalf("close first sqlite backend: %v", err)
	}

	_, _, secondCloser, err := NewSQLiteBackends(dsn)
	if err != nil {
		t.Fatalf("second NewSQLiteBackends() error = %v", err)
	}

	if err := secondCloser.Close(); err != nil {
		t.Fatalf("close second sqlite backend: %v", err)
	}
}

func TestSQLiteShortURLCounterNextInitializesMissingCounterRow(t *testing.T) {
	t.Parallel()

	_, counter, closer, err := NewSQLiteBackends("sqlite3:///" + t.TempDir() + "/counter.sqlite3")
	if err != nil {
		t.Fatalf("NewSQLiteBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close sqlite backend: %v", closeErr)
		}
	}()

	if _, err := counter.db.ExecContext(
		context.Background(),
		`DELETE FROM counters WHERE name = ?`,
		shortURLCounterName,
	); err != nil {
		t.Fatalf("delete counter row: %v", err)
	}

	next, err := counter.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}

	if next != 1 {
		t.Fatalf("Next() = %d, want 1", next)
	}
}

func TestSQLiteShortURLStorageExpireTimeRoundTrip(t *testing.T) {
	t.Parallel()

	storage, _, closer, err := NewSQLiteBackends("sqlite3:///" + t.TempDir() + "/expiry.sqlite3")
	if err != nil {
		t.Fatalf("NewSQLiteBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close sqlite backend: %v", closeErr)
		}
	}()

	ctx := context.Background()
	expiry := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	entry := service.ShortURL{
		ID:          "expirytest",
		OriginalURL: "https://example.com/expiry",
		ExpireTime:  &expiry,
		CreateTime:  time.Now().UTC().Truncate(time.Second),
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

// TestSQLiteShortURLStorageCaseSensitiveIDs verifies that the SQLite storage
// treats IDs as case-sensitive: "abcdef" and "ABCDEF" must be distinct rows.
// SQLite TEXT columns use binary collation for equality comparisons by default.
func TestSQLiteShortURLStorageCaseSensitiveIDs(t *testing.T) {
	t.Parallel()

	storage, _, closer, err := NewSQLiteBackends(
		"sqlite3:///" + t.TempDir() + "/casesensitive.sqlite3",
	)
	if err != nil {
		t.Fatalf("NewSQLiteBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close sqlite backend: %v", closeErr)
		}
	}()

	ctx := context.Background()
	lower := "abc"
	upper := "ABC"

	lowerEntry := service.ShortURL{
		ID:          lower,
		OriginalURL: sqliteLowerURL,
		CreateTime:  time.Now().UTC().Truncate(time.Second),
	}
	upperEntry := service.ShortURL{
		ID:          upper,
		OriginalURL: sqliteUpperURL,
		CreateTime:  time.Now().UTC().Truncate(time.Second),
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

func TestSQLiteShortURLStorageNilExpireTimeRoundTrip(t *testing.T) {
	t.Parallel()

	storage, _, closer, err := NewSQLiteBackends("sqlite3:///" + t.TempDir() + "/noexpiry.sqlite3")
	if err != nil {
		t.Fatalf("NewSQLiteBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close sqlite backend: %v", closeErr)
		}
	}()

	ctx := context.Background()
	entry := service.ShortURL{
		ID:          "noexpiry1",
		OriginalURL: "https://example.com/noexpiry",
		ExpireTime:  nil,
		CreateTime:  time.Now().UTC().Truncate(time.Second),
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

	if got.ExpireTime != nil {
		t.Fatalf("GetByID() ExpireTime = %v, want nil", got.ExpireTime)
	}
}
