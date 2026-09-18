// Copyright 2026 The MinURL Authors

package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"strconv"

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

type errorResponse struct {
	status int
	msg    string
}

// errorResponses is the one place a service error is given an HTTP status. Operations list
// which of these errors they return, through register; they never pick a status themselves.
var errorResponses = map[error]errorResponse{
	service.ErrShortURLNotFound:   {http.StatusNotFound, "short URL not found"},
	service.ErrShortURLIDConflict: {http.StatusConflict, "short URL ID already exists"},
	// The API sets no length limit, but a storage backend may have one.
	service.ErrOriginalURLTooLong: {http.StatusRequestEntityTooLarge, "original URL is too long for storage"},
}

// bodyReadErrors are the statuses returned before the handler runs on an operation with a
// request body: missing body or malformed JSON (or a corrupt gzip body), body read timeout,
// body of MaxBodyBytes or more, and a Content-Type or Content-Encoding the server cannot read.
// huma and middleware.RequestDecompress both answer them as ErrorModel bodies, and neither
// passes through a handler, so no service error can produce them. huma also appends 422 (for
// any operation with a body or path params) and 500 itself; that 500 also answers a body the
// client cut short, with the read error (e.g. "unexpected EOF") attached, since it never
// reaches toHTTPError.
var bodyReadErrors = []int{
	http.StatusBadRequest,
	http.StatusRequestTimeout,
	http.StatusRequestEntityTooLarge,
	http.StatusUnsupportedMediaType,
}

// operation is the part of huma.Operation register can vouch for. register builds the
// huma.Operation itself, so a field that changes which statuses huma returns cannot be set
// behind its back: Errors and Responses (register derives them), MaxBodyBytes and
// BodyReadTimeout (a negative value disables the 413 and 408 it publishes), SkipValidateBody,
// SkipValidateParams and RejectUnknownQueryParameters (they remove or add a 422), Middlewares
// (they can write any status without passing through toHTTPError), RequestBody (a body huma
// may never read) and Hidden (keeps the operation out of the document). Add a field only once
// it is clear it changes none of the statuses register publishes.
//
// There is no tags field: every operation is a short URL operation so far, and register tags
// it shortURLTag. Add one with the first operation that needs another tag.
type operation struct {
	// id is the OpenAPI operationId. It must be unique across the API.
	id string

	// method is the HTTP method, e.g. http.MethodGet.
	method string

	// path is the route, with {name} for each path parameter.
	path string

	// summary is the one-line description published in the document.
	summary string

	// defaultStatus is the success status; 0 keeps huma's default.
	defaultStatus int

	// errs lists every error the handler returns. It is published through errorResponses and
	// is the only list the handler's errors are converted through.
	errs []error
}

// register registers h as op, with op.errs as the single source of its error responses, so a
// handler cannot return an error status the OpenAPI document does not list. h returns service
// errors as they are.
//
// The published list must be exhaustive for what the server returns: what op.errs cannot
// prove is that the service still returns each error, so
// TestRegisterDeclaresEveryReachableErrorStatus drives a request to every listed error.
func register[I, O any](api huma.API, op operation, h func(context.Context, *I) (*O, error)) {
	// A Body field is what makes huma limit, read and parse the body, the four ways
	// bodyReadErrors lists. A RawBody fails differently: it gets no size limit and is never
	// parsed (no 413 or 415), so register refuses it rather than publish statuses it cannot
	// return.
	inputType := reflect.TypeFor[I]()
	if _, ok := inputType.FieldByName("RawBody"); ok {
		panic(fmt.Sprintf("handler: %s: register only knows the error statuses of a Body input, not a RawBody", op.id))
	}

	var framework []int
	if _, ok := inputType.FieldByName("Body"); ok {
		framework = bodyReadErrors
	}

	responses := map[string]*huma.Response{}

	huma.Register(api, huma.Operation{
		OperationID:   op.id,
		Method:        op.method,
		Path:          op.path,
		Summary:       op.summary,
		Tags:          []string{shortURLTag},
		DefaultStatus: op.defaultStatus,
		Errors:        errorStatuses(framework, op.errs),
		Responses:     responses,
	}, func(ctx context.Context, input *I) (*O, error) {
		out, err := h(ctx, input)
		if err != nil {
			return nil, toHTTPError(ctx, err, op.errs)
		}

		return out, nil
	})

	// huma adds `default` only to an operation that lists no errors, and never removes one
	// already there. Kept, a generated client still decodes an ErrorModel for a status nobody
	// lists, such as a gateway's 502. huma publishes the very map it was handed, so describing
	// `default` after registration returns can reuse the content huma wrote for 500 — the status
	// errorStatuses always lists — instead of mirroring huma's unexported defineErrors. The two
	// share that content map: changing what one of them describes changes the other.
	responses["default"] = &huma.Response{
		Description: "Error",
		Content:     responses[strconv.Itoa(http.StatusInternalServerError)].Content,
	}
}

// errorStatuses returns the statuses to publish for an operation: those returned before
// the handler runs, the 500 toHTTPError can always return, and the status of every error
// the operation lists. The 500 keeps the list non-empty, which is what makes huma add its
// own 422 and 500. It panics on an error errorResponses does not know, so a missing entry
// fails the first test that builds an API instead of reaching a client as an undocumented
// 500.
func errorStatuses(framework []int, errs []error) []int {
	statuses := append(slices.Clone(framework), http.StatusInternalServerError)

	for _, target := range errs {
		resp, ok := errorResponses[target]
		if !ok {
			panic(fmt.Sprintf("handler: no errorResponses entry for %q", target))
		}

		statuses = append(statuses, resp.status)
	}

	slices.Sort(statuses)

	return slices.Compact(statuses)
}

// toHTTPError converts err into the response for the first of errs it matches. Anything
// else is a 500 whose body carries no detail: the store wraps driver errors, which name
// tables and columns, and huma serializes any attached error into the response. It is
// logged instead, with the request's attributes, since the log is the only record.
//
// A known error the operation did not list is a 500 too, since answering with its real
// status would publish nothing about it. The log marks it unlisted_error, because that is
// a missing list entry rather than a server fault. A client that went away is not a server
// fault either, so context.Canceled is logged at WARN; the response is still a 500 in the
// access log.
func toHTTPError(ctx context.Context, err error, errs []error) error {
	for _, target := range errs {
		if errors.Is(err, target) {
			resp := errorResponses[target]

			return huma.NewError(resp.status, resp.msg)
		}
	}

	unlisted := false

	for known := range errorResponses {
		if errors.Is(err, known) {
			unlisted = true

			break
		}
	}

	level := slog.LevelError
	if errors.Is(err, context.Canceled) {
		level = slog.LevelWarn
	}

	slog.With(middleware.AttrsToAny(middleware.LoggerAttrsFromContext(ctx))...).
		Log(ctx, level, "request failed", "error", err, "unlisted_error", unlisted)

	return huma.NewError(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
}
