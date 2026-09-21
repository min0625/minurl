// Copyright 2026 The MinURL Authors

package testhelpers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"

	"github.com/danielgtaylor/huma/v2"
)

// StringSliceContains checks if a string slice contains a specific value.
func StringSliceContains(values []string, want string) bool {
	return slices.Contains(values, want)
}

// Operations returns every operation in doc, keyed by method and path, e.g.
// "GET /api/v1/urls/{id}".
func Operations(doc *huma.OpenAPI) map[string]*huma.Operation {
	ops := map[string]*huma.Operation{}

	for path, item := range doc.Paths {
		for method, op := range map[string]*huma.Operation{
			http.MethodGet: item.Get, http.MethodPut: item.Put, http.MethodPost: item.Post,
			http.MethodDelete: item.Delete, http.MethodOptions: item.Options, http.MethodHead: item.Head,
			http.MethodPatch: item.Patch, http.MethodTrace: item.Trace,
		} {
			if op != nil {
				ops[method+" "+path] = op
			}
		}
	}

	return ops
}

// ErrorResponses returns the codes of op's error responses, `default` included, sorted.
func ErrorResponses(op *huma.Operation) []string {
	var codes []string

	for code := range op.Responses {
		if code == "default" || code[0] >= '4' {
			codes = append(codes, code)
		}
	}

	slices.Sort(codes)

	return codes
}

// DecodeErrorModel decodes body as a single ErrorModel, rejecting any field the published
// schema does not declare (it sets additionalProperties: false) and anything after it.
// The schema declares $schema, which huma adds to its own errors and ErrorModel lacks.
func DecodeErrorModel(body []byte) (huma.ErrorModel, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()

	var m struct {
		Schema string `json:"$schema"`
		huma.ErrorModel
	}

	if err := dec.Decode(&m); err != nil {
		return huma.ErrorModel{}, err
	}

	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return huma.ErrorModel{}, errors.New("data after the ErrorModel")
	}

	return m.ErrorModel, nil
}
