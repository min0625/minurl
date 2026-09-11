package store //nolint:testpackage // White-box tests validate internal MySQL helpers.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/min0625/minurl/internal/service"
)

const (
	mysqlLowerURL = "https://example.com/lower"
	mysqlUpperURL = "https://example.com/upper"
)

// mysqlDSNFields is the part of the driver config parseMySQLDSN is responsible for.
// Comparing the whole *mysqldriver.Config would drag in the driver's own defaults.
type mysqlDSNFields struct {
	User   string
	Passwd string
	Addr   string
	DBName string
	TLS    string
}

func newMySQLDSNFields(cfg *mysqldriver.Config) mysqlDSNFields {
	if cfg == nil {
		return mysqlDSNFields{}
	}

	return mysqlDSNFields{
		User:   cfg.User,
		Passwd: cfg.Passwd,
		Addr:   cfg.Addr,
		DBName: cfg.DBName,
		TLS:    cfg.TLSConfig,
	}
}

func TestParseMySQLDSN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		dsn     string
		wantErr bool
		want    mysqlDSNFields
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
			want: mysqlDSNFields{User: "user", Passwd: "pass", Addr: "localhost:3306", DBName: "dbname"},
		},
		{ //nolint:gosec // test credentials in DSN
			name: "basic DSN with port",
			dsn:  "mysql://user:pass@localhost:3307/dbname",
			want: mysqlDSNFields{User: "user", Passwd: "pass", Addr: "localhost:3307", DBName: "dbname"},
		},
		{ //nolint:gosec // test credentials in DSN
			name: "DSN with extra params",
			dsn:  "mysql://user:pass@localhost:3306/dbname?tls=skip-verify",
			want: mysqlDSNFields{
				User: "user", Passwd: "pass", Addr: "localhost:3306", DBName: "dbname", TLS: "skip-verify",
			},
		},
		{ //nolint:gosec // test credentials in DSN
			name: "DSN with special chars in password",
			dsn:  "mysql://user:p%40ss@localhost/dbname",
			want: mysqlDSNFields{User: "user", Passwd: "p@ss", Addr: "localhost:3306", DBName: "dbname"},
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

			if fields := newMySQLDSNFields(got); fields != tc.want {
				t.Fatalf("parseMySQLDSN(%q) = %+v, want %+v", tc.dsn, fields, tc.want)
			}
		})
	}
}

func TestParseMySQLDSNEnforcesParseTime(t *testing.T) {
	t.Parallel()

	cfg, err := parseMySQLDSN("mysql://user:pass@localhost/dbname")
	if err != nil {
		t.Fatalf("parseMySQLDSN() error = %v", err)
	}

	if !cfg.ParseTime {
		t.Fatalf("ParseTime = false, want true: time.Time values require parseTime=true")
	}

	if cfg.Loc == nil || cfg.Loc.String() != "UTC" {
		t.Fatalf("Loc = %v, want UTC", cfg.Loc)
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

	// ON DUPLICATE KEY UPDATE must leave the stored row alone.
	got, ok, err := storage.GetByID(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("GetByID() = %v, %v, %v, want found", got, ok, err)
	}

	if got.OriginalURL != entry.OriginalURL {
		t.Fatalf("OriginalURL = %q, want %q: the duplicate overwrote the row", got.OriginalURL, entry.OriginalURL)
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
		OriginalURL: mysqlLowerURL,
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}
	upperEntry := service.ShortURL{
		ID:          upper,
		OriginalURL: mysqlUpperURL,
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

// TestParseMySQLDSNKeepsDriverFlagsOutOfConfig pins that a caller-supplied driver-level
// flag never becomes a driver flag. parseMySQLDSN leaves unrecognised params in
// cfg.Params, which the driver sends as SET k = v, so the server rejects the connection
// (see TestNewMySQLBackendsRejectsDriverLevelFlagsInDSN). Formatting the config back into
// a DSN string would instead promote every one of these names into the flag it shadows:
// multiStatements allows stacked queries on the application pool, clientFoundRows makes
// CreateIfAbsent report an existing id as newly created, allowAllFiles lets LOAD DATA
// LOCAL INFILE read arbitrary files from this machine, interpolateParams switches every
// query off server-side parameter binding, and allowCleartextPasswords sends the password
// in the clear.
func TestParseMySQLDSNKeepsDriverFlagsOutOfConfig(t *testing.T) {
	t.Parallel()

	flags := map[string]func(*mysqldriver.Config) bool{
		"multiStatements":         func(c *mysqldriver.Config) bool { return c.MultiStatements },
		"clientFoundRows":         func(c *mysqldriver.Config) bool { return c.ClientFoundRows },
		"allowAllFiles":           func(c *mysqldriver.Config) bool { return c.AllowAllFiles },
		"interpolateParams":       func(c *mysqldriver.Config) bool { return c.InterpolateParams },
		"allowCleartextPasswords": func(c *mysqldriver.Config) bool { return c.AllowCleartextPasswords },
		"allowOldPasswords":       func(c *mysqldriver.Config) bool { return c.AllowOldPasswords },
		"columnsWithAlias":        func(c *mysqldriver.Config) bool { return c.ColumnsWithAlias },
	}

	for name, isSet := range flags {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfg, err := parseMySQLDSN("mysql://user:pass@localhost/dbname?" + name + "=true")
			if err != nil {
				t.Fatalf("parseMySQLDSN() error = %v", err)
			}

			if isSet(cfg) {
				t.Fatalf("%s was promoted to a driver flag, want it left off", name)
			}

			if cfg.Params[name] != "true" {
				t.Fatalf("cfg.Params[%q] = %q, want %q: it must stay a server variable so the "+
					"server rejects it", name, cfg.Params[name], "true")
			}
		})
	}
}

// TestNewMySQLBackendsRejectsDriverLevelFlagsInDSN is the end-to-end half of the check
// above: a driver flag left in cfg.Params reaches the server as an unknown system
// variable, so the pool refuses to open instead of quietly running with the flag on.
func TestNewMySQLBackendsRejectsDriverLevelFlagsInDSN(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	_, _, closer, err := NewMySQLBackends(testMySQLDSN+"?multiStatements=true", DBPoolConfig{})
	if err == nil {
		_ = closer.Close()

		t.Fatal("NewMySQLBackends() error = nil, want the server to reject multiStatements")
	}

	if !strings.Contains(err.Error(), "Unknown system variable") {
		t.Fatalf("NewMySQLBackends() error = %v, want an unknown system variable error", err)
	}
}

// TestMySQLShortURLStorageHasNoSecondUniqueKey pins the invariant CreateIfAbsent's
// ON DUPLICATE KEY UPDATE depends on. ODKU fires on any unique key, not just the primary
// one, so a second UNIQUE index would make a violation of it return created=false and the
// service would answer 409 for a row that never conflicted on id. Measured against MySQL
// 8.4 with a second unique index in place: CreateIfAbsent returns (false, nil), while
// SQLite's scoped ON CONFLICT (id) correctly returns the constraint error.
func TestMySQLShortURLStorageHasNoSecondUniqueKey(t *testing.T) {
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

	rows, err := storage.db.QueryContext(
		context.Background(),
		`SELECT DISTINCT index_name FROM information_schema.statistics
		 WHERE table_schema = DATABASE() AND table_name = 'short_urls' AND non_unique = 0`,
	)
	if err != nil {
		t.Fatalf("query unique indexes: %v", err)
	}

	defer rows.Close() //nolint:errcheck // best-effort close

	var names []string

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index name: %v", err)
		}

		names = append(names, name)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("iterate unique indexes: %v", err)
	}

	if len(names) != 1 || names[0] != "PRIMARY" {
		t.Fatalf("unique indexes on short_urls = %v, want only [PRIMARY]: ON DUPLICATE KEY "+
			"UPDATE would report a violation of the new index as an id conflict", names)
	}
}

// TestMySQLOriginalURLColumnMatchesMaxBytes pins mysqlMaxOriginalURLBytes to the column it
// claims to describe. The constant is a hand-copied TEXT capacity, and nothing else ties the
// two together: widening original_url to MEDIUMTEXT in a later migration would leave the
// constant silently too strict, answering 413 for URLs the column can now hold.
func TestMySQLOriginalURLColumnMatchesMaxBytes(t *testing.T) {
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

	// character_octet_length, not character_maximum_length: the constant counts bytes.
	var octetLength int64

	if err := storage.db.QueryRowContext(
		context.Background(),
		`SELECT character_octet_length FROM information_schema.columns
		 WHERE table_schema = DATABASE() AND table_name = 'short_urls'
		   AND column_name = 'original_url'`,
	).Scan(&octetLength); err != nil {
		t.Fatalf("query original_url column length: %v", err)
	}

	if octetLength != mysqlMaxOriginalURLBytes {
		t.Fatalf("original_url holds %d bytes but mysqlMaxOriginalURLBytes = %d: "+
			"CreateIfAbsent rejects URLs the column can store", octetLength, mysqlMaxOriginalURLBytes)
	}
}

// TestMySQLShortURLStorageRejectsOversizedOriginalURL pins that a URL longer than the
// TEXT column is reported as service.ErrOriginalURLTooLong. The API sets no length limit,
// so without it the request answers 500 instead of 413. No database is needed: the length
// is checked before the insert.
func TestMySQLShortURLStorageRejectsOversizedOriginalURL(t *testing.T) {
	t.Parallel()

	storage := &MySQLShortURLStorage{}

	entry := service.ShortURL{
		ID:          "m-long",
		OriginalURL: "https://example.com/" + strings.Repeat("a", mysqlMaxOriginalURLBytes),
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}

	created, err := storage.CreateIfAbsent(context.Background(), entry)
	if created {
		t.Fatalf("CreateIfAbsent() = true, want false")
	}

	if !errors.Is(err, service.ErrOriginalURLTooLong) {
		t.Fatalf("CreateIfAbsent() error = %v, want %v", err, service.ErrOriginalURLTooLong)
	}
}

// TestMySQLShortURLStorageRejectsOversizedOriginalURLWithoutStrictMode is the end-to-end
// half of the check above. MySQL only raises ER_DATA_TOO_LONG in strict SQL mode, so a
// server configured without STRICT_TRANS_TABLES used to store the row truncated and
// report it as created, leaving the response carrying a URL the redirect could not serve.
func TestMySQLShortURLStorageRejectsOversizedOriginalURLWithoutStrictMode(t *testing.T) {
	t.Parallel()
	skipIfNoIntegration(t)

	// sql_mode is not a driver flag, so it reaches the server as SET sql_mode = ''.
	storage, _, closer, err := NewMySQLBackends(testMySQLDSN+"?sql_mode=%27%27", DBPoolConfig{})
	if err != nil {
		t.Fatalf("NewMySQLBackends() error = %v", err)
	}

	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			t.Fatalf("close mysql backend: %v", closeErr)
		}
	}()

	entry := service.ShortURL{
		ID:          fmt.Sprintf("m-long-%d", time.Now().UnixNano()),
		OriginalURL: "https://example.com/" + strings.Repeat("a", mysqlMaxOriginalURLBytes),
		CreateTime:  time.Now().UTC().Truncate(time.Microsecond),
	}

	created, err := storage.CreateIfAbsent(context.Background(), entry)
	if created {
		t.Fatalf("CreateIfAbsent() = true, want false")
	}

	if !errors.Is(err, service.ErrOriginalURLTooLong) {
		t.Fatalf("CreateIfAbsent() error = %v, want %v", err, service.ErrOriginalURLTooLong)
	}

	if _, ok, err := storage.GetByID(context.Background(), entry.ID); err != nil || ok {
		t.Fatalf("GetByID() found = %v, err = %v, want not found: a truncated row was stored", ok, err)
	}
}
