package store //nolint:testpackage // White-box tests validate internal SQLite helpers.

import (
	"context"
	"testing"
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
