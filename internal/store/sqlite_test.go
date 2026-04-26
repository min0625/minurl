package store //nolint:testpackage // White-box tests validate internal SQLite helpers.

import (
	"context"
	"testing"
)

func TestOpenSQLiteDBUsesSingleConnectionPool(t *testing.T) {
	t.Parallel()

	db, err := openSQLiteDB(t.TempDir() + "/pool.sqlite3")
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

	_, counter, closer, err := NewSQLiteBackends(t.TempDir() + "/counter.sqlite3")
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
