// Copyright 2024 The MinURL Authors

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/min0625/minurl/internal/service"
)

// SQLiteShortURLStorage is a SQLite-backed storage implementation.
type SQLiteShortURLStorage struct {
	db *sql.DB
}

// CreateIfAbsent stores the entry if the ID does not already exist.
// Returns true if the entry was inserted, false if it already existed.
func (s *SQLiteShortURLStorage) CreateIfAbsent(
	ctx context.Context,
	entry service.ShortURL,
) (bool, error) {
	result, err := s.db.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO short_urls (id, original_url, create_time) VALUES (?, ?, ?)`,
		entry.ID,
		entry.OriginalURL,
		entry.CreateTime.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return false, fmt.Errorf("insert short url: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}

	return rows > 0, nil
}

// GetByID returns the short URL with the given ID.
func (s *SQLiteShortURLStorage) GetByID(
	ctx context.Context,
	id string,
) (service.ShortURL, bool, error) {
	var entry service.ShortURL

	var createTimeStr string

	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, original_url, create_time FROM short_urls WHERE id = ?`,
		id,
	).Scan(&entry.ID, &entry.OriginalURL, &createTimeStr)

	if errors.Is(err, sql.ErrNoRows) {
		return service.ShortURL{}, false, nil
	}

	if err != nil {
		return service.ShortURL{}, false, fmt.Errorf("query short url: %w", err)
	}

	entry.CreateTime, err = time.Parse(time.RFC3339Nano, createTimeStr)
	if err != nil {
		return service.ShortURL{}, false, fmt.Errorf("parse create_time %q: %w", createTimeStr, err)
	}

	return entry, true, nil
}

// Close releases the database connection.
func (s *SQLiteShortURLStorage) Close() error {
	return s.db.Close()
}
