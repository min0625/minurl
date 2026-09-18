// Copyright 2026 The MinURL Authors

package handler

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/min0625/minurl/internal/service"
	"github.com/min0625/minurl/internal/testhelpers"
)

type (
	bodyInput struct {
		Body struct {
			Name string `json:"name"`
		}
	}
	// promotedBodyInput reaches Body through an embedded struct; huma and register both find
	// it with reflect's FieldByName, which follows promotion.
	promotedBodyInput struct{ bodyInput }
	pathInput         struct {
		ID string `path:"id"`
	}
	noInput      struct{}
	rawBodyInput struct{ RawBody []byte }
	emptyOutput  struct{}
)

func newRegisterTestAPI() huma.API {
	return humachi.New(chi.NewRouter(), huma.DefaultConfig("test", "0.0.0"))
}

func handlerFor[I any]() func(context.Context, *I) (*emptyOutput, error) {
	return func(context.Context, *I) (*emptyOutput, error) { return &emptyOutput{}, nil }
}

// registeredOperation returns op as the document holds it.
func registeredOperation(t *testing.T, api huma.API, op operation) *huma.Operation {
	t.Helper()

	registered := testhelpers.Operations(api.OpenAPI())[op.method+" "+op.path]
	if registered == nil {
		t.Fatalf("%s %s not in the document", op.method, op.path)
	}

	return registered
}

// TestRegisterBuildsTheHumaOperation pins the field-by-field copy from operation into the
// huma.Operation register builds, and the `default` response it adds. Either lost changes only
// the published document, which no request-driven test would notice.
func TestRegisterBuildsTheHumaOperation(t *testing.T) {
	t.Parallel()

	api := newRegisterTestAPI()
	op := operation{
		id: "op", method: http.MethodGet, path: "/op", summary: "Summary", defaultStatus: http.StatusAccepted,
	}
	register(api, op, handlerFor[noInput]())

	got := registeredOperation(t, api, op)
	if got.OperationID != "op" || got.Summary != "Summary" || !slices.Equal(got.Tags, []string{shortURLTag}) {
		t.Errorf("operation id, summary, tags = %q, %q, %v, want %q, %q, %v",
			got.OperationID, got.Summary, got.Tags, "op", "Summary", []string{shortURLTag})
	}

	if got.DefaultStatus != http.StatusAccepted || got.Responses["202"] == nil {
		t.Errorf("default status = %d with responses %v, want 202",
			got.DefaultStatus, slices.Collect(maps.Keys(got.Responses)))
	}

	// register describes `default` itself; it must match what huma writes for a listed status.
	if def, want := got.Responses["default"], got.Responses["500"]; def == nil || want == nil ||
		!reflect.DeepEqual(def.Content, want.Content) {
		t.Errorf("default response = %+v, want the content of the 500 response %+v", def, want)
	}
}

// TestRegisterPublishesErrorStatusesForEachInputShape pins what register adds for input
// shapes the real operations do not cover. The reachability test in short_url_test.go proves
// the Body statuses are actually returned; this one guards the reflection that decides them.
func TestRegisterPublishesErrorStatusesForEachInputShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		register func(huma.API, operation)
		op       operation
		want     []string
	}{
		{
			name: "body with a listed error",
			register: func(api huma.API, op operation) {
				register(api, op, handlerFor[bodyInput]())
			},
			op: operation{
				id: "op", method: http.MethodPost, path: "/body", errs: []error{service.ErrShortURLIDConflict},
			},
			want: []string{"400", "408", "409", "413", "415", "422", "500", "default"},
		},
		{
			name: "promoted body",
			register: func(api huma.API, op operation) {
				register(api, op, handlerFor[promotedBodyInput]())
			},
			op:   operation{id: "op", method: http.MethodPost, path: "/promoted"},
			want: []string{"400", "408", "413", "415", "422", "500", "default"},
		},
		{
			// With no errors and no body, Operation.Errors would be empty and huma would skip its
			// own 422 and 500; register's fixed 500 keeps both.
			name: "path params and no errors",
			register: func(api huma.API, op operation) {
				register(api, op, handlerFor[pathInput]())
			},
			op:   operation{id: "op", method: http.MethodGet, path: "/items/{id}"},
			want: []string{"422", "500", "default"},
		},
		{
			name: "no input and no errors",
			register: func(api huma.API, op operation) {
				register(api, op, handlerFor[noInput]())
			},
			op:   operation{id: "op", method: http.MethodGet, path: "/none"},
			want: []string{"500", "default"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := newRegisterTestAPI()
			tt.register(api, tt.op)

			if got := testhelpers.ErrorResponses(registeredOperation(t, api, tt.op)); !slices.Equal(got, tt.want) {
				t.Fatalf("error responses = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRegisterRefusesOperationsItCannotDescribe pins the registration-time panics that stand
// in for a published status list register cannot vouch for. Fields of huma.Operation that
// would change the statuses (RequestBody, MaxBodyBytes, Middlewares, …) need no case: operation
// has no way to set them.
func TestRegisterRefusesOperationsItCannotDescribe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		register  func(huma.API)
		wantPanic string
	}{
		{
			name: "error without an errorResponses entry",
			register: func(api huma.API) {
				op := operation{id: "op", method: http.MethodGet, path: "/op", errs: []error{errors.New("unmapped")}}
				register(api, op, handlerFor[noInput]())
			},
			wantPanic: "no errorResponses entry",
		},
		{
			// huma reads a RawBody without a size limit and never parses it, so it cannot
			// return 413 or 415.
			name: "raw body",
			register: func(api huma.API) {
				register(api, operation{id: "op", method: http.MethodPost, path: "/op"}, handlerFor[rawBodyInput]())
			},
			wantPanic: "RawBody",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Any panic is not enough: huma panics on its own for a malformed operation.
			defer func() {
				if got := fmt.Sprint(recover()); !strings.Contains(got, tt.wantPanic) {
					t.Fatalf("register panicked with %q, want a panic mentioning %q", got, tt.wantPanic)
				}
			}()

			tt.register(newRegisterTestAPI())
		})
	}
}
