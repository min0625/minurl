// Copyright 2024 The MinURL Authors

// Package testhelpers provides test utilities and mock implementations for unit testing.
package testhelpers

import (
	"context"
	"sync/atomic"
)

// Counter is a test implementation of the ShortURLCounter interface.
// It provides a monotonically increasing sequence.
type Counter struct {
	value atomic.Uint32
}

// NewCounter creates a new test counter starting at 0.
func NewCounter() *Counter {
	return &Counter{}
}

// Next returns the next counter value.
func (c *Counter) Next(ctx context.Context) (uint32, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	return c.value.Add(1), nil
}

// GetValue returns the current counter value without incrementing.
func (c *Counter) GetValue() uint32 {
	return c.value.Load()
}
