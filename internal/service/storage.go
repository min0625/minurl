// Copyright 2024 The MinURL Authors

package service

import "context"

// ShortURLStorage describes storage operations required by ShortURLService.
type ShortURLStorage interface {
	CreateIfAbsent(ctx context.Context, entry ShortURL) (bool, error)
	GetByID(ctx context.Context, id string) (ShortURL, bool, error)
}
