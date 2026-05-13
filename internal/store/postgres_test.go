package store //nolint:testpackage // White-box tests validate internal PostgreSQL helpers.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/min0625/minurl/internal/service"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// testPostgresDSN is set by TestMain when INTEGRATION_TEST=1 and holds the
// connection string for the single shared PostgreSQL container used by all
// tests in this package.
var testPostgresDSN string

// testMySQLDSN is set by TestMain when INTEGRATION_TEST=1 and holds the
// connection string for the single shared MySQL container used by all MySQL
// tests in this package.
var testMySQLDSN string

// TestMain manages the lifecycle of shared database containers for all
// integration tests in this package. Run with:
//
//	INTEGRATION_TEST=1 go test ./internal/store/...
//	make test INTEGRATION_TEST=1
func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		os.Exit(m.Run())
	}

	ctx := context.Background()

	// Start PostgreSQL container.
	pgCtr, err := postgres.Run(
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

	pgDSN, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: get postgres connection string: %v\n", err)

		_ = pgCtr.Terminate(ctx)

		os.Exit(1)
	}

	testPostgresDSN = pgDSN

	// Start MySQL container.
	mysqlCtr, err := mysql.Run(
		ctx,
		"mysql:8.4",
		mysql.WithDatabase("testdb"),
		mysql.WithUsername("testuser"),
		mysql.WithPassword("testpass"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: start mysql container: %v\n", err)

		_ = pgCtr.Terminate(ctx)

		os.Exit(1)
	}

	mysqlHost, err := mysqlCtr.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: get mysql host: %v\n", err)

		_ = pgCtr.Terminate(ctx)
		_ = mysqlCtr.Terminate(ctx)

		os.Exit(1)
	}

	mysqlPort, err := mysqlCtr.MappedPort(ctx, "3306/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: get mysql port: %v\n", err)

		_ = pgCtr.Terminate(ctx)
		_ = mysqlCtr.Terminate(ctx)

		os.Exit(1)
	}

	testMySQLDSN = fmt.Sprintf(
		"mysql://testuser:testpass@%s:%s/testdb",
		mysqlHost,
		mysqlPort.Port(),
	)

	code := m.Run()

	_ = pgCtr.Terminate(ctx)
	_ = mysqlCtr.Terminate(ctx)

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
		t.Skip("set INTEGRATION_TEST=1 to run DB integration tests (requires Docker)")
	}
}

func TestOpenPostgresDBSetsConnectionPool(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	db, err := openPostgresDB(testPostgresDSN, DBPoolConfig{MaxOpenConns: 25, MaxIdleConns: 5})
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

	storage, _, closer, err := NewPostgresBackends(testPostgresDSN, DBPoolConfig{})
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

	storage, _, closer, err := NewPostgresBackends(testPostgresDSN, DBPoolConfig{})
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

	storage, _, closer, err := NewPostgresBackends(testPostgresDSN, DBPoolConfig{})
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

	storage, _, closer, err := NewPostgresBackends(testPostgresDSN, DBPoolConfig{})
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

	_, counter, closer, err := NewPostgresBackends(testPostgresDSN, DBPoolConfig{})
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

	storage, counter, closer, err := NewPostgresBackends(testPostgresDSN, DBPoolConfig{})
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

func TestPostgresShortURLStorageExpireTimeRoundTrip(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, _, closer, err := NewPostgresBackends(testPostgresDSN, DBPoolConfig{})
	if err != nil {
		t.Fatalf("NewPostgresBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close postgres backend: %v", closeErr)
		}
	}()

	ctx := context.Background()
	expiry := time.Date(2030, 6, 15, 9, 0, 0, 0, time.UTC)
	entry := service.ShortURL{
		ID:          "pgexpiry01",
		OriginalURL: "https://example.com/pgexpiry",
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

// TestPostgresShortURLStorageCaseSensitiveIDs verifies that the PostgreSQL storage
// treats IDs as case-sensitive: "abcdef" and "ABCDEF" must be distinct rows.
// PostgreSQL text types use case-sensitive comparison by default.
func TestPostgresShortURLStorageCaseSensitiveIDs(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, _, closer, err := NewPostgresBackends(testPostgresDSN, DBPoolConfig{})
	if err != nil {
		t.Fatalf("NewPostgresBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close postgres backend: %v", closeErr)
		}
	}()

	ctx := context.Background()
	base := fmt.Sprintf("pgcs%d", time.Now().UnixNano())
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

func TestPostgresShortURLStorageNilExpireTimeRoundTrip(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	storage, _, closer, err := NewPostgresBackends(testPostgresDSN, DBPoolConfig{})
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
		ID:          "pgnoexpiry1",
		OriginalURL: "https://example.com/pgnoexpiry",
		ExpireTime:  nil,
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

	if got.ExpireTime != nil {
		t.Fatalf("GetByID() ExpireTime = %v, want nil", got.ExpireTime)
	}
}
