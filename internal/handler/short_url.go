// Copyright 2026 The MinURL Authors

// Package handler registers HTTP route handlers for the MinURL service.
package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/min0625/minurl/internal/middleware"
	"github.com/min0625/minurl/internal/service"
)

const shortURLTag = "ShortURL"

// Request validation only holds if huma validates the value encoding/json then stores.
// huma validates the exact key, but encoding/json matches keys case-insensitively and
// keeps the last match, so {"id":"abc","ID":"bad*id"} would store the unvalidated "ID".
// Strict casing rejects every key the schema does not declare verbatim. Set in init so
// it is in place before any API is built, and never written while requests run.
func init() {
	huma.ValidateStrictCasing = true //nolint:reassign // huma exposes this setting only as a package var.
}

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

				// The API sets no length limit, but a storage backend may have one.
				// err is not attached: huma serializes it into the response body, and
				// the store wraps the driver error, which names the table column.
				if errors.Is(err, service.ErrOriginalURLTooLong) {
					return nil, huma.Error413RequestEntityTooLarge(
						"original URL is too long for storage",
					)
				}

				return nil, internalServerError(ctx, "failed to create short URL", err)
			}

			return &shortURLOutput{Body: *entry}, nil
		},
	)
}

// internalServerError logs err and returns a 500 carrying msg alone. err is never attached:
// huma serializes an attached error into the response body, and the store wraps driver
// errors that name tables and columns. The log line, with the request's attributes, is the
// only record of the failure. A client that went away is not a server fault, so
// context.Canceled is logged at WARN; the response is still a 500 in the access log.
func internalServerError(ctx context.Context, msg string, err error) error {
	level := slog.LevelError
	if errors.Is(err, context.Canceled) {
		level = slog.LevelWarn
	}

	slog.With(middleware.AttrsToAny(middleware.LoggerAttrsFromContext(ctx))...).
		Log(ctx, level, msg, "error", err)

	return huma.Error500InternalServerError(msg)
}

// registerGetShortURLRoute registers the get short URL endpoint on the given API.
// The handler implements the full business logic using the provided service.
func registerGetShortURLRoute(api huma.API, svc service.ShortURLServicer) {
	huma.Register(
		api,
		getShortURLOperation,
		func(ctx context.Context, input *shortURLIDInput) (*shortURLOutput, error) {
			entry, err := svc.Get(ctx, input.ID)
			if err != nil {
				if errors.Is(err, service.ErrShortURLNotFound) {
					return nil, huma.Error404NotFound("short URL not found")
				}

				return nil, internalServerError(ctx, "failed to get short URL", err)
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
		func(ctx context.Context, input *shortURLIDInput) (*redirectOutput, error) {
			entry, err := svc.Get(ctx, input.ID)
			if err != nil {
				if errors.Is(err, service.ErrShortURLNotFound) {
					return nil, huma.Error404NotFound("short URL not found")
				}

				return nil, internalServerError(ctx, "failed to get short URL", err)
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
//
// svc may be nil when the API is built only to produce the OpenAPI document:
// registration never calls it. Every handler dereferences it, so a router
// registered with a nil service must not be served.
func Register(api huma.API, svc service.ShortURLServicer) {
	registerCreateShortURLRoute(api, svc)
	registerGetShortURLRoute(api, svc)
	registerRedirectRoute(api, svc)
}
