// Copyright 2024 The MinURL Authors

package service

import (
	"context"

	"github.com/min0625/minurl/internal/model"
)

// ShortURLStorage describes storage operations required by ShortURLService.
type ShortURLStorage interface {
	CreateIfAbsent(ctx context.Context, entry model.ShortURL) (bool, error)
	GetByID(ctx context.Context, id string) (model.ShortURL, bool, error)
}
