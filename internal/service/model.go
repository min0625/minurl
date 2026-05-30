// Copyright 2024 The MinURL Authors

package service

import "time"

// ShortURL represents a shortened URL resource.
type ShortURL struct {
	ID          string     `doc:"Unique id"                                   json:"id,omitempty"          validate:"omitempty,shortid"`
	OriginalURL string     `doc:"Original URL to shorten"                     json:"original_url"          validate:"required,url"      required:"true"`
	ExpireTime  *time.Time `doc:"Expiration time; omit or null for permanent" json:"expire_time,omitempty" validate:"omitempty"                         nullable:"true"`
	CreateTime  time.Time  `doc:"Creation time"                               json:"create_time,omitempty"                                                              readOnly:"true"`
}
