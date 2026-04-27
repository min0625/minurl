// Copyright 2024 The MinURL Authors

// Package handler registers HTTP route handlers for the MinURL service.
package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-playground/validator/v10"
	"github.com/min0625/minurl/internal/model"
	"github.com/min0625/minurl/internal/service"
)

var requestValidator = newRequestValidator()

func newRequestValidator() *validator.Validate {
	v := validator.New()
	if err := v.RegisterValidation("shortid", func(fl validator.FieldLevel) bool {
		return service.IsValidShortURLID(fl.Field().String()) == nil
	}); err != nil {
		panic(err)
	}

	return v
}

func validateRequest(req any, msg string) []error {
	if err := requestValidator.Struct(req); err != nil {
		return []error{huma.Error400BadRequest(msg, err)}
	}

	return nil
}

// ShortURLService defines the minimal behavior required by HTTP handlers.
// Note: This interface is defined locally (rather than imported from service package)
// to allow the handler to evolve independently and define only the operations it needs.
// This pattern supports loose coupling between the HTTP layer and service layer.
type ShortURLService interface {
	Create(ctx context.Context, entry model.ShortURL) (*model.ShortURL, error)
	Get(ctx context.Context, id string) (*model.ShortURL, bool, error)
}

type createShortURLInput struct {
	Body model.ShortURL `validate:"required"`
}

var _ huma.Resolver = (*createShortURLInput)(nil)

func (in *createShortURLInput) Resolve(huma.Context) []error {
	return validateRequest(in, "invalid create short URL request")
}

type shortURLOutput struct {
	Body model.ShortURL
}

type getShortURLInput struct {
	ID string `path:"id" doc:"Short URL identifier" validate:"required,shortid"`
}

var _ huma.Resolver = (*getShortURLInput)(nil)

func (in *getShortURLInput) Resolve(huma.Context) []error {
	return validateRequest(in, "invalid get short URL request")
}

// registerCreateShortURLRoute registers the create short URL endpoint on the given API.
// The handler implements the full business logic using the provided service.
func registerCreateShortURLRoute(api huma.API, svc ShortURLService) {
	huma.Register(api, huma.Operation{
		OperationID: "create-short-url",
		Method:      http.MethodPost,
		Path:        "/api/v1/urls",
		Summary:     "Create a short URL",
		Tags:        []string{"ShortURL"},
	}, func(ctx context.Context, input *createShortURLInput) (*shortURLOutput, error) {
		entry, err := svc.Create(ctx, input.Body)
		if err != nil {
			if errors.Is(err, service.ErrShortURLIDConflict) {
				return nil, huma.Error409Conflict("short URL ID already exists", err)
			}

			return nil, huma.Error500InternalServerError("failed to create short URL", err)
		}

		return &shortURLOutput{Body: *entry}, nil
	})
}

// registerGetShortURLRoute registers the get short URL endpoint on the given API.
// The handler implements the full business logic using the provided service.
func registerGetShortURLRoute(api huma.API, svc ShortURLService) {
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

// Register registers all short URL routes onto the given API with the provided service.
func Register(api huma.API, svc ShortURLService) {
	registerCreateShortURLRoute(api, svc)
	registerGetShortURLRoute(api, svc)
}

// RegisterOpenAPI registers all short URL routes onto the given API for OpenAPI schema generation.
// This variant does not require a service implementation and is suitable for documentation generation.
func RegisterOpenAPI(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-short-url",
		Method:      http.MethodPost,
		Path:        "/api/v1/urls",
		Summary:     "Create a short URL",
		Tags:        []string{"ShortURL"},
	}, func(_ context.Context, _ *createShortURLInput) (*shortURLOutput, error) {
		return nil, huma.Error500InternalServerError("not implemented", nil)
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-short-url",
		Method:      http.MethodGet,
		Path:        "/api/v1/urls/{id}",
		Summary:     "Get a short URL by ID",
		Tags:        []string{"ShortURL"},
	}, func(_ context.Context, _ *getShortURLInput) (*shortURLOutput, error) {
		return nil, huma.Error500InternalServerError("not implemented", nil)
	})
}
