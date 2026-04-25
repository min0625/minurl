// Copyright 2024 The MinURL Authors

package service

import (
	"context"
	"fmt"
	"sync/atomic"
)

const maxUint32 = ^uint32(0)

// ShortURLCounter describes counter operations required by ShortURLService.
type ShortURLCounter interface {
	Next(ctx context.Context) (uint32, error)
}

// InMemoryShortURLCounter provides a lock-free monotonic uint32 counter.
type InMemoryShortURLCounter struct {
	value atomic.Uint32
}

// NewInMemoryShortURLCounter creates the default in-memory counter backend.
func NewInMemoryShortURLCounter() *InMemoryShortURLCounter {
	return &InMemoryShortURLCounter{}
}

// Next returns the next monotonic sequence value or an error when exhausted/canceled.
func (c *InMemoryShortURLCounter) Next(ctx context.Context) (uint32, error) {
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		current := c.value.Load()
		if current == maxUint32 {
			return 0, fmt.Errorf("short id sequence exhausted")
		}

		next := current + 1
		if c.value.CompareAndSwap(current, next) {
			return next, nil
		}
	}
}
