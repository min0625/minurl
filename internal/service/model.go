// Copyright 2024 The MinURL Authors

package service

import "time"

// ShortURL represents a shortened URL resource.
type ShortURL struct {
	ID          string    `json:"id,omitempty"          validate:"omitempty,shortid" doc:"Unique id"`
	OriginalURL string    `json:"original_url"          validate:"required,url"      doc:"Original URL to shorten" required:"true"`
	CreateTime  time.Time `json:"create_time,omitempty"                              doc:"Creation time"                           readOnly:"true"`
}
