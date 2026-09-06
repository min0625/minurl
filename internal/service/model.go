// Copyright 2024 The MinURL Authors

package service

import (
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// ShortURL represents a shortened URL resource.
type ShortURL struct {
	ID          string      `json:"id,omitzero"          doc:"Unique id"                                   maxLength:"12" pattern:"^[123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz]+$" patternDescription:"Base58 characters"`
	OriginalURL OriginalURL `json:"original_url"         doc:"Original URL to shorten (http or https)"                                                                                                                                     required:"true"`
	ExpireTime  *time.Time  `json:"expire_time,omitzero" doc:"Expiration time; omit or null for permanent"                                                                                                                                                 nullable:"true"`
	CreateTime  time.Time   `json:"create_time,omitzero" doc:"Creation time"                                                                                                                                                                                               readOnly:"true"`
}

// OriginalURL is the URL a short link points at. The type carries its own request
// validation: the schema constraints huma checks before unmarshalling, and the
// scheme/host/userinfo rules it cannot express, checked right after.
type OriginalURL string

var (
	_ huma.SchemaProvider   = OriginalURL("")
	_ huma.ResolverWithPath = OriginalURL("")
)

// Schema publishes the constraints huma checks against the parsed JSON, before it
// unmarshals into the struct. It reads MaxOriginalURLLen directly, which a maxLength
// struct tag cannot do.
func (OriginalURL) Schema(huma.Registry) *huma.Schema {
	minLen, maxLen := 1, MaxOriginalURLLen

	return &huma.Schema{
		Type:      "string",
		MinLength: &minLen,
		MaxLength: &maxLen,
	}
}

// Resolve runs on request input only. huma calls it for every field of this type,
// including inside slices, and supplies the error location.
func (u OriginalURL) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	if u == "" {
		// A missing field reaches here as the zero value; required and minLength
		// have already reported it.
		return nil
	}

	if err := IsValidOriginalURL(string(u)); err != nil {
		return []error{&huma.ErrorDetail{
			Location: prefix.String(),
			Message:  err.Error(),
			Value:    string(u),
		}}
	}

	return nil
}
