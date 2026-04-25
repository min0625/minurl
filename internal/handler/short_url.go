// Copyright 2024 The MinURL Authors

// Package handler registers HTTP route handlers for the MinURL service.
package handler

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/min0625/minurl/internal/model"
	"github.com/min0625/minurl/internal/service"
)

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
func Register(api huma.API, svc *service.ShortURLService) {
	huma.Register(api, huma.Operation{
		OperationID: "create-short-url",
		Method:      http.MethodPost,
		Path:        "/short-urls",
		Summary:     "Create a short URL",
		Tags:        []string{"ShortURL"},
	}, func(_ context.Context, input *createShortURLInput) (*shortURLOutput, error) {
		entry, err := svc.Create(input.Body.OriginalURL)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to create short URL", err)
		}

		return &shortURLOutput{Body: *entry}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-short-url",
		Method:      http.MethodGet,
		Path:        "/short-urls/{id}",
		Summary:     "Get a short URL by ID",
		Tags:        []string{"ShortURL"},
	}, func(_ context.Context, input *getShortURLInput) (*shortURLOutput, error) {
		entry, ok := svc.Get(input.ID)
		if !ok {
			return nil, huma.Error404NotFound("short URL not found")
		}

		return &shortURLOutput{Body: *entry}, nil
	})
}
