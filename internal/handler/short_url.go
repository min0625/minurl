// Copyright 2024 The MinURL Authors

// Package handler registers HTTP route handlers for the MinURL service.
package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/min0625/minurl/internal/model"
)

var errShortURLServiceUnavailable = errors.New("short url service unavailable")

// ShortURLService defines the minimal behavior required by HTTP handlers.
type ShortURLService interface {
	Create(ctx context.Context, originalURL string) (*model.ShortURL, error)
	Get(ctx context.Context, id string) (*model.ShortURL, bool, error)
}

type createShortURLInput struct {
	Body struct {
		OriginalURL string `json:"original_url" required:"true" doc:"Original URL to shorten"`
	}
}

type shortURLOutput struct {
	Body model.ShortURL
}

type getShortURLInput struct {
	ID string `path:"id" doc:"Short URL identifier"`
}

// Register registers all short URL routes onto the given API.
func Register(api huma.API, svc ShortURLService) {
	if svc == nil {
		svc = noopShortURLService{}
	}

	huma.Register(api, huma.Operation{
		OperationID: "create-short-url",
		Method:      http.MethodPost,
		Path:        "/api/v1/urls",
		Summary:     "Create a short URL",
		Tags:        []string{"ShortURL"},
	}, func(ctx context.Context, input *createShortURLInput) (*shortURLOutput, error) {
		entry, err := svc.Create(ctx, input.Body.OriginalURL)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to create short URL", err)
		}

		return &shortURLOutput{Body: *entry}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-short-url",
		Method:      http.MethodGet,
		Path:        "/api/v1/urls/{id}",
		Summary:     "Get a short URL by ID",
		Tags:        []string{"ShortURL"},
	}, func(ctx context.Context, input *getShortURLInput) (*shortURLOutput, error) {
		entry, ok, err := svc.Get(ctx, input.ID)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to get short URL", err)
		}

		if !ok {
			return nil, huma.Error404NotFound("short URL not found")
		}

		return &shortURLOutput{Body: *entry}, nil
	})
}

type noopShortURLService struct{}

func (noopShortURLService) Create(context.Context, string) (*model.ShortURL, error) {
	return nil, errShortURLServiceUnavailable
}

func (noopShortURLService) Get(context.Context, string) (*model.ShortURL, bool, error) {
	return nil, false, errShortURLServiceUnavailable
}
