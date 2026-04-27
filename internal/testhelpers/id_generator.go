// Copyright 2024 The MinURL Authors

package testhelpers

import "sync/atomic"

// IDGenerator is a test implementation that generates fixed IDs for testing.
// It tracks the number of times Generate was called.
type IDGenerator struct {
	id    string
	calls atomic.Uint32
}

// NewIDGenerator creates a test ID generator that always returns the specified ID.
func NewIDGenerator(id string) *IDGenerator {
	return &IDGenerator{
		id: id,
	}
}

// Generate returns the configured fixed ID.
func (g *IDGenerator) Generate(_ uint32) string {
	g.calls.Add(1)
	return g.id
}

// CallCount returns the number of times Generate was called.
func (g *IDGenerator) CallCount() uint32 {
	return g.calls.Load()
}
