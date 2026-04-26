// Copyright 2024 The MinURL Authors

package service

import (
	"context"
)

// ShortURLCounter describes counter operations required by ShortURLService.
type ShortURLCounter interface {
	Next(ctx context.Context) (uint32, error)
}
