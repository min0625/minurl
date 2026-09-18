// Copyright 2026 The MinURL Authors

// Package handler registers HTTP route handlers for the MinURL service.
//
// Operations are declared here; the registration machinery they go through — operation,
// register, errorResponses and toHTTPError — lives in register.go.
package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/min0625/minurl/internal/service"
)

type createShortURLInput struct {
	Body service.ShortURL
}

type shortURLOutput struct {
	Body service.ShortURL
}

// shortURLIDInput is the input for every operation addressed by an {id} path param.
// The get and redirect operations take the same parameter under the same rules, so they
// share one declaration; huma keys operations off huma.Operation, not the input type.
type shortURLIDInput struct {
	ID string `path:"id" doc:"Short URL identifier" maxLength:"12" pattern:"^[123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz]+$" patternDescription:"Base58 characters"`
}

// redirectOutput has no Status field: the 302 comes from the operation's defaultStatus, the
// status the document publishes.
type redirectOutput struct {
	Location string `doc:"URL to redirect to" header:"Location"`
}

// registerCreateShortURLRoute registers the create short URL endpoint on the given API.
func registerCreateShortURLRoute(api huma.API, svc service.ShortURLServicer) {
	register(
		api,
		operation{
			id:      "create-short-url",
			method:  http.MethodPost,
			path:    "/api/v1/urls",
			summary: "Create a short URL",
			errs:    []error{service.ErrShortURLIDConflict, service.ErrOriginalURLTooLong},
		},
		func(ctx context.Context, input *createShortURLInput) (*shortURLOutput, error) {
			entry, err := svc.Create(ctx, input.Body)
			if err != nil {
				return nil, err
			}

			return &shortURLOutput{Body: *entry}, nil
		},
	)
}

// registerGetShortURLRoute registers the get short URL endpoint on the given API.
func registerGetShortURLRoute(api huma.API, svc service.ShortURLServicer) {
	register(
		api,
		operation{
			id:      "get-short-url",
			method:  http.MethodGet,
			path:    "/api/v1/urls/{id}",
			summary: "Get a short URL by ID",
			errs:    []error{service.ErrShortURLNotFound},
		},
		func(ctx context.Context, input *shortURLIDInput) (*shortURLOutput, error) {
			entry, err := svc.Get(ctx, input.ID)
			if err != nil {
				return nil, err
			}

			return &shortURLOutput{Body: *entry}, nil
		},
	)
}

// registerRedirectRoute registers the redirect endpoint on the given Huma API.
// The handler retrieves a short URL and performs an HTTP 302 redirect to the original URL.
func registerRedirectRoute(api huma.API, svc service.ShortURLServicer) {
	register(
		api,
		operation{
			id:            "redirect-short-url",
			method:        http.MethodGet,
			path:          "/api/v1/urls/{id}:redirect",
			summary:       "Redirect to original URL",
			defaultStatus: http.StatusFound,
			errs:          []error{service.ErrShortURLNotFound},
		},
		func(ctx context.Context, input *shortURLIDInput) (*redirectOutput, error) {
			entry, err := svc.Get(ctx, input.ID)
			if err != nil {
				return nil, err
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

				return nil, service.ErrShortURLNotFound
			}

			return &redirectOutput{Location: string(entry.OriginalURL)}, nil
		},
	)
}

// Register registers all short URL routes onto the given API with the provided service.
//
// svc may be nil when the API is built only to produce the OpenAPI document:
// registration never calls it. Every handler dereferences it, so a router
// registered with a nil service must not be served.
func Register(api huma.API, svc service.ShortURLServicer) {
	registerCreateShortURLRoute(api, svc)
	registerGetShortURLRoute(api, svc)
	registerRedirectRoute(api, svc)
}
