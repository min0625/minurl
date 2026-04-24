// Copyright 2024 The MinURL Authors

package service_test

import (
	"testing"

	"github.com/min0625/minurl/internal/service"
)

func TestShortURLServiceCreateAndGet(t *testing.T) {
	t.Parallel()

	svc := service.NewShortURLService()

	entry, err := svc.Create("https://example.org/path")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if entry.ID == "" {
		t.Fatal("Create() returned empty ID")
	}

	if entry.CreateTime.IsZero() {
		t.Fatal("Create() returned zero CreateTime")
	}

	got, ok := svc.Get(entry.ID)
	if !ok {
		t.Fatalf("Get(%q) returned ok = false", entry.ID)
	}

	if got.OriginalURL != "https://example.org/path" {
		t.Fatalf(
			"Get(%q) original_url = %q, want %q",
			entry.ID,
			got.OriginalURL,
			"https://example.org/path",
		)
	}

	if got.ID != entry.ID {
		t.Fatalf("Get(%q) id = %q, want %q", entry.ID, got.ID, entry.ID)
	}

	if _, ok := svc.Get("missing"); ok {
		t.Fatal("Get(missing) returned ok = true, want false")
	}
}

func TestShortURLServiceGetReturnsCopy(t *testing.T) {
	t.Parallel()

	svc := service.NewShortURLService()

	entry, err := svc.Create("https://example.org/original")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, ok := svc.Get(entry.ID)
	if !ok {
		t.Fatalf("Get(%q) returned ok = false", entry.ID)
	}

	got.OriginalURL = "https://example.org/mutated"

	gotAgain, ok := svc.Get(entry.ID)
	if !ok {
		t.Fatalf("Get(%q) second read returned ok = false", entry.ID)
	}

	if gotAgain.OriginalURL != "https://example.org/original" {
		t.Fatalf(
			"stored value mutated via returned pointer: got %q, want %q",
			gotAgain.OriginalURL,
			"https://example.org/original",
		)
	}
}
