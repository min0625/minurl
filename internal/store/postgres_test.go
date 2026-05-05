package store //nolint:testpackage // White-box tests validate internal PostgreSQL helpers.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/min0625/minurl/internal/model"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// skipIfNoIntegration skips the test unless the INTEGRATION_TEST environment
// variable is set to "1". This allows running integration tests in CI and via explicit
// go.mod while avoiding Docker dependency in normal `go test ./...` runs.
// It also skips gracefully when Docker is unavailable (e.g. in some CI envs).
func skipIfNoIntegration(t *testing.T) {
	t.Helper()

	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("set INTEGRATION_TEST=1 to run PostgreSQL integration tests (requires Docker)")
	}

	testcontainers.SkipIfProviderIsNotHealthy(t)
}

// startPostgresContainer starts a postgres:16-alpine container, registers
// cleanup via t.Cleanup, and returns a DSN suitable for openPostgresDB.
func startPostgresContainer(t *testing.T) string {
	t.Helper()

	ctx := context.Background()

	ctr, err := postgres.Run(
		ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		postgres.BasicWaitStrategies(),
	)

	testcontainers.CleanupContainer(t, ctr)

	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get postgres connection string: %v", err)
	}

	return dsn
}

func TestOpenPostgresDBSetsConnectionPool(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	dsn := startPostgresContainer(t)

	db, err := openPostgresDB(dsn)
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
}

func TestPostgresShortURLStorageCreateIfAbsent(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	dsn := startPostgresContainer(t)

	storage, _, closer, err := NewPostgresBackends(dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close postgres backend: %v", closeErr)
		}
	}()

	entry := model.ShortURL{
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

	dsn := startPostgresContainer(t)

	storage, _, closer, err := NewPostgresBackends(dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close postgres backend: %v", closeErr)
		}
	}()

	entry := model.ShortURL{
		ID:          "dup001",
		OriginalURL: "https://example.com/first",
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}

	if _, err := storage.CreateIfAbsent(context.Background(), entry); err != nil {
		t.Fatalf("first CreateIfAbsent() error = %v", err)
	}

	duplicate := model.ShortURL{
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

	dsn := startPostgresContainer(t)

	storage, _, closer, err := NewPostgresBackends(dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close postgres backend: %v", closeErr)
		}
	}()

	want := model.ShortURL{
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

	dsn := startPostgresContainer(t)

	storage, _, closer, err := NewPostgresBackends(dsn)
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
	t.Parallel()
	skipIfNoIntegration(t)

	dsn := startPostgresContainer(t)

	_, counter, closer, err := NewPostgresBackends(dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close postgres backend: %v", closeErr)
		}
	}()

	ctx := context.Background()

	for i := range uint32(3) {
		want := i + 1

		got, err := counter.Next(ctx)
		if err != nil {
			t.Fatalf("Next() call %d error = %v", i+1, err)
		}

		if got != want {
			t.Fatalf("Next() call %d = %d, want %d", i+1, got, want)
		}
	}
}

func TestNewPostgresBackends(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	dsn := startPostgresContainer(t)

	storage, counter, closer, err := NewPostgresBackends(dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close postgres backend: %v", closeErr)
		}
	}()

	ctx := context.Background()

	entry := model.ShortURL{
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

	n, err := counter.Next(ctx)
	if err != nil {
		t.Fatalf("counter.Next() error = %v", err)
	}

	if n != 1 {
		t.Fatalf("counter.Next() = %d, want 1", n)
	}
}
