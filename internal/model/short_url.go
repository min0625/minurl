// Copyright 2024 The MinURL Authors

// Package model defines the domain types for the MinURL service.
package model

import "time"

// ShortURL represents a shortened URL resource.
type ShortURL struct {
	ID          string    `json:"id"           readOnly:"true" doc:"Unique identifier"`
	OriginalURL string    `json:"original_url"                 doc:"Original URL to shorten"`
	CreateTime  time.Time `json:"create_time"  readOnly:"true" doc:"Creation time"`
}
