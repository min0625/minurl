package store //nolint:testpackage // White-box tests validate internal PostgreSQL helpers.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/min0625/minurl/internal/service"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// testPostgresDSN is set by TestMain when INTEGRATION_TEST=1 and holds the
// connection string for the single shared PostgreSQL container used by all
// tests in this package.
var testPostgresDSN string

// TestMain manages the lifecycle of a single shared PostgreSQL container for
// all integration tests in this package. Run with:
//
//	INTEGRATION_TEST=1 go test ./internal/store/...
//	make test INTEGRATION_TEST=1
func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		os.Exit(m.Run())
	}

	ctx := context.Background()

	ctr, err := postgres.Run(
		ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: start postgres container: %v\n", err)
		os.Exit(1)
	}

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: get postgres connection string: %v\n", err)

		_ = ctr.Terminate(ctx)

		os.Exit(1)
	}

	testPostgresDSN = dsn
	code := m.Run()
	_ = ctr.Terminate(ctx)

	os.Exit(code)
}

// skipIfNoIntegration skips the test unless the INTEGRATION_TEST environment
// variable is set to "1". Run integration tests with:
//
//	INTEGRATION_TEST=1 go test ./internal/store/...
//	make test INTEGRATION_TEST=1
func skipIfNoIntegration(t *testing.T) {
	t.Helper()

	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("set INTEGRATION_TEST=1 to run PostgreSQL integration tests (requires Docker)")
	}
}

func TestOpenPostgresDBSetsConnectionPool(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	db, err := openPostgresDB(testPostgresDSN)
	if err != nil {
		t.Fatalf("openPostgresDB() error = %v", err)
	}

	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Fatalf("close db: %v", closeErr)
		}
	}()

	if got := db.Stats().MaxOpenConnections; got != 25 {
		t.Fatalf("MaxOpenConnections = %d, want 25", got)
	}

	// Verify MaxIdleConns=5: acquire 10 connections then release all;
	// the pool should retain at most 5 idle connections.
	ctx := context.Background()
	conns := make([]*sql.Conn, 10)

	for i := range conns {
		conns[i], err = db.Conn(ctx)
		if err != nil {
			t.Fatalf("db.Conn()[%d] error = %v", i, err)
		}
	}

	for _, c := range conns {
		if closeErr := c.Close(); closeErr != nil {
			t.Fatalf("close conn: %v", closeErr)
		}
	}

	if got := db.Stats().Idle; got > 5 {
		t.Fatalf("Idle = %d, want <= 5 (MaxIdleConns=5)", got)
	}
}

func TestPostgresShortURLStorageCreateIfAbsent(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, _, closer, err := NewPostgresBackends(testPostgresDSN)
	if err != nil {
		t.Fatalf("NewPostgresBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close postgres backend: %v", closeErr)
		}
	}()

	entry := service.ShortURL{
		ID:          "abc123",
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

func TestPostgresShortURLStorageCreateIfAbsentConflict(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, _, closer, err := NewPostgresBackends(testPostgresDSN)
	if err != nil {
		t.Fatalf("NewPostgresBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close postgres backend: %v", closeErr)
		}
	}()

	entry := service.ShortURL{
		ID:          "dup001",
		OriginalURL: "https://example.com/first",
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}

	if _, err := storage.CreateIfAbsent(context.Background(), entry); err != nil {
		t.Fatalf("first CreateIfAbsent() error = %v", err)
	}

	duplicate := service.ShortURL{
		ID:          "dup001",
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

func TestPostgresShortURLStorageGetByID(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, _, closer, err := NewPostgresBackends(testPostgresDSN)
	if err != nil {
		t.Fatalf("NewPostgresBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close postgres backend: %v", closeErr)
		}
	}()

	want := service.ShortURL{
		ID:          "get001",
		OriginalURL: "https://example.com/get",
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}

	if _, err := storage.CreateIfAbsent(context.Background(), want); err != nil {
		t.Fatalf("CreateIfAbsent() error = %v", err)
	}

	got, found, err := storage.GetByID(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if !found {
		t.Fatalf("GetByID() found = false, want true")
	}

	if got.ID != want.ID {
		t.Fatalf("GetByID() ID = %q, want %q", got.ID, want.ID)
	}

	if got.OriginalURL != want.OriginalURL {
		t.Fatalf("GetByID() OriginalURL = %q, want %q", got.OriginalURL, want.OriginalURL)
	}

	if !got.CreateTime.Equal(want.CreateTime) {
		t.Fatalf("GetByID() CreateTime = %v, want %v", got.CreateTime, want.CreateTime)
	}
}

func TestPostgresShortURLStorageGetByIDNotFound(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, _, closer, err := NewPostgresBackends(testPostgresDSN)
	if err != nil {
		t.Fatalf("NewPostgresBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close postgres backend: %v", closeErr)
		}
	}()

	_, found, err := storage.GetByID(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if found {
		t.Fatalf("GetByID() found = true, want false for missing entry")
	}
}

func TestPostgresShortURLCounterNext(t *testing.T) {
	skipIfNoIntegration(t)

	_, counter, closer, err := NewPostgresBackends(testPostgresDSN)
	if err != nil {
		t.Fatalf("NewPostgresBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close postgres backend: %v", closeErr)
		}
	}()

	ctx := context.Background()

	// Verify the counter returns monotonically increasing values (+1 each call).
	// Do not assert the exact starting value because the shared container's
	// counter table may already hold rows from other tests in this package.
	prev, err := counter.Next(ctx)
	if err != nil {
		t.Fatalf("Next() call 1 error = %v", err)
	}

	if prev == 0 {
		t.Fatalf("Next() call 1 = 0, want > 0")
	}

	for call := range uint32(2) {
		got, err := counter.Next(ctx)
		if err != nil {
			t.Fatalf("Next() call %d error = %v", call+2, err)
		}

		if got != prev+1 {
			t.Fatalf("Next() call %d = %d, want %d (prev+1)", call+2, got, prev+1)
		}

		prev = got
	}
}

func TestNewPostgresBackends(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, counter, closer, err := NewPostgresBackends(testPostgresDSN)
	if err != nil {
		t.Fatalf("NewPostgresBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close postgres backend: %v", closeErr)
		}
	}()

	ctx := context.Background()

	entry := service.ShortURL{
		ID:          "happy001",
		OriginalURL: "https://example.com/happy",
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}

	created, err := storage.CreateIfAbsent(ctx, entry)
	if err != nil {
		t.Fatalf("CreateIfAbsent() error = %v", err)
	}

	if !created {
		t.Fatalf("CreateIfAbsent() = false, want true")
	}

	// Verify the counter is functional; do not assert the exact value because
	// the shared container may have had prior counter increments.
	n, err := counter.Next(ctx)
	if err != nil {
		t.Fatalf("counter.Next() error = %v", err)
	}

	if n == 0 {
		t.Fatalf("counter.Next() = 0, want > 0")
	}
}
