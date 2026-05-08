// Copyright 2024 The MinURL Authors

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
)

// SQLiteShortURLCounter is a SQLite-backed counter implementation.
type SQLiteShortURLCounter struct {
	db *sql.DB
}

// Close releases the database connection.
func (c *SQLiteShortURLCounter) Close() error {
	return c.db.Close()
}

// Next returns the next monotonic sequence value.
func (c *SQLiteShortURLCounter) Next(ctx context.Context) (uint64, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}

		tx, err := c.db.BeginTx(ctx, nil)
		if err != nil {
			return 0, fmt.Errorf("begin tx: %w", err)
		}

		next, committed, err := c.nextInTx(ctx, tx)
		if err != nil {
			_ = tx.Rollback()

			return 0, err
		}

		if !committed {
			_ = tx.Rollback()

			continue
		}

		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("commit tx: %w", err)
		}

		return next, nil
	}
}

func (c *SQLiteShortURLCounter) nextInTx(
	ctx context.Context,
	tx *sql.Tx,
) (uint64, bool, error) {
	var current uint64

	err := tx.QueryRowContext(
		ctx,
		`SELECT value FROM counters WHERE name = ?`,
		shortURLCounterName,
	).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		result, insertErr := tx.ExecContext(
			ctx,
			`INSERT OR IGNORE INTO counters (name, value) VALUES (?, 1)`,
			shortURLCounterName,
		)
		if insertErr != nil {
			return 0, false, fmt.Errorf("initialize counter row: %w", insertErr)
		}

		affectedRows, affectedErr := result.RowsAffected()
		if affectedErr != nil {
			return 0, false, fmt.Errorf("rows affected: %w", affectedErr)
		}

		if affectedRows == 0 {
			return 0, false, nil
		}

		return 1, true, nil
	}

	if err != nil {
		return 0, false, fmt.Errorf("read counter value: %w", err)
	}

	if current == math.MaxUint64 {
		return 0, false, fmt.Errorf("short id sequence exhausted")
	}

	next := current + 1

	result, err := tx.ExecContext(
		ctx,
		`UPDATE counters SET value = ? WHERE name = ? AND value = ?`,
		next,
		shortURLCounterName,
		current,
	)
	if err != nil {
		return 0, false, fmt.Errorf("update counter value: %w", err)
	}

	affectedRows, err := result.RowsAffected()
	if err != nil {
		return 0, false, fmt.Errorf("rows affected: %w", err)
	}

	if affectedRows == 0 {
		return 0, false, nil
	}

	return next, true, nil
}
