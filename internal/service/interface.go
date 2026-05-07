// Copyright 2024 The MinURL Authors

package service

import (
	"context"

	"github.com/go-playground/validator/v10"
)

// ShortURLServicer is the contract for short URL business logic operations.
// Implementations must contain no HTTP or transport concerns.
type ShortURLServicer interface {
	// Create creates a new short URL and returns it.
	// If entry.ID is provided, it will be used as the short URL identifier.
	// Returns ErrShortURLIDConflict if the given ID already exists.
	Create(ctx context.Context, entry ShortURL) (*ShortURL, error)

	// Get retrieves the short URL with the given ID.
	// Returns (nil, false, nil) when the ID is not found.
	Get(ctx context.Context, id string) (*ShortURL, bool, error)
}

// Compile-time assertion: ShortURLService must satisfy ShortURLServicer.
var _ ShortURLServicer = (*ShortURLService)(nil)

// RegisterValidations registers service-level validation rules onto v.
// Call this once during application startup so that validator tags such as
// "shortid" reflect the same constraints enforced by the service layer.
func RegisterValidations(v *validator.Validate) error {
	return v.RegisterValidation("shortid", func(fl validator.FieldLevel) bool {
		return IsValidShortURLID(fl.Field().String()) == nil
	})
}
