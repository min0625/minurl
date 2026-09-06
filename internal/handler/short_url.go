// Copyright 2024 The MinURL Authors

// Package handler registers HTTP route handlers for the MinURL service.
package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/min0625/minurl/internal/service"
)

const shortURLTag = "ShortURL"

type createShortURLInput struct {
	Body service.ShortURL
}

type shortURLOutput struct {
	Body service.ShortURL
}

type getShortURLInput struct {
	ID string `path:"id" doc:"Short URL identifier" maxLength:"12" pattern:"^[123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz]+$" patternDescription:"Base58 characters"`
}

var createShortURLOperation = huma.Operation{
	OperationID: "create-short-url",
	Method:      http.MethodPost,
	Path:        "/api/v1/urls",
	Summary:     "Create a short URL",
	Tags:        []string{shortURLTag},
}

var getShortURLOperation = huma.Operation{
	OperationID: "get-short-url",
	Method:      http.MethodGet,
	Path:        "/api/v1/urls/{id}",
	Summary:     "Get a short URL by ID",
	Tags:        []string{shortURLTag},
}

type redirectInput struct {
	ID string `path:"id" doc:"Short URL identifier" maxLength:"12" pattern:"^[123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz]+$" patternDescription:"Base58 characters"`
}

type redirectOutput struct {
	Status   int
	Location string `doc:"URL to redirect to" header:"Location"`
}

var redirectShortURLOperation = huma.Operation{
	OperationID:   "redirect-short-url",
	Method:        http.MethodGet,
	Path:          "/api/v1/urls/{id}:redirect",
	Summary:       "Redirect to original URL",
	Tags:          []string{shortURLTag},
	DefaultStatus: http.StatusFound,
}

// registerCreateShortURLRoute registers the create short URL endpoint on the given API.
// The handler implements the full business logic using the provided service.
func registerCreateShortURLRoute(api huma.API, svc service.ShortURLServicer) {
	huma.Register(
		api,
		createShortURLOperation,
		func(ctx context.Context, input *createShortURLInput) (*shortURLOutput, error) {
			entry, err := svc.Create(ctx, input.Body)
			if err != nil {
				if errors.Is(err, service.ErrShortURLIDConflict) {
					return nil, huma.Error409Conflict("short URL ID already exists", err)
				}

				return nil, huma.Error500InternalServerError("failed to create short URL", err)
			}

			return &shortURLOutput{Body: *entry}, nil
		},
	)
}

// registerGetShortURLRoute registers the get short URL endpoint on the given API.
// The handler implements the full business logic using the provided service.
func registerGetShortURLRoute(api huma.API, svc service.ShortURLServicer) {
	huma.Register(
		api,
		getShortURLOperation,
		func(ctx context.Context, input *getShortURLInput) (*shortURLOutput, error) {
			entry, ok, err := svc.Get(ctx, input.ID)
			if err != nil {
				return nil, huma.Error500InternalServerError("failed to get short URL", err)
			}

			if !ok {
				return nil, huma.Error404NotFound("short URL not found")
			}

			return &shortURLOutput{Body: *entry}, nil
		},
	)
}

// registerRedirectRoute registers the redirect endpoint on the given Huma API.
// The handler retrieves a short URL and performs an HTTP 302 redirect to the original URL.
func registerRedirectRoute(api huma.API, svc service.ShortURLServicer) {
	huma.Register(
		api,
		redirectShortURLOperation,
		func(ctx context.Context, input *redirectInput) (*redirectOutput, error) {
			entry, ok, err := svc.Get(ctx, input.ID)
			if err != nil {
				return nil, huma.Error500InternalServerError("failed to get short URL", err)
			}

			if !ok {
				return nil, huma.Error404NotFound("short URL not found")
			}

			// Entries created before the scheme allowlist existed may hold a
			// javascript:/data:/file: URL, so refuse to hand one back as a Location.
			if err := service.IsValidOriginalURL(string(entry.OriginalURL)); err != nil {
				slog.WarnContext(
					ctx,
					"stored original URL is not a valid http(s) URL",
					"id", input.ID,
					"error", err,
				)

				return nil, huma.Error404NotFound("short URL not found")
			}

			return &redirectOutput{
				Status:   http.StatusFound,
				Location: string(entry.OriginalURL),
			}, nil
		},
	)
}

// Register registers all short URL routes onto the given API with the provided service.
func Register(api huma.API, svc service.ShortURLServicer) {
	registerCreateShortURLRoute(api, svc)
	registerGetShortURLRoute(api, svc)
	registerRedirectRoute(api, svc)
}

// RegisterOpenAPI registers all short URL routes onto the given API for OpenAPI schema generation.
// This variant does not require a service implementation and is suitable for documentation generation.
func RegisterOpenAPI(api huma.API) {
	huma.Register(
		api,
		createShortURLOperation,
		func(_ context.Context, _ *createShortURLInput) (*shortURLOutput, error) {
			return nil, huma.Error500InternalServerError("not implemented", nil)
		},
	)

	huma.Register(
		api,
		getShortURLOperation,
		func(_ context.Context, _ *getShortURLInput) (*shortURLOutput, error) {
			return nil, huma.Error500InternalServerError("not implemented", nil)
		},
	)

	huma.Register(
		api,
		redirectShortURLOperation,
		func(_ context.Context, _ *redirectInput) (*redirectOutput, error) {
			return nil, huma.Error500InternalServerError("not implemented", nil)
		},
	)
}
