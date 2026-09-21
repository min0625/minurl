// Copyright 2026 The MinURL Authors

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/min0625/minurl/internal/middleware"
)

// TestWriteErrorEncodesLikeHuma pins that WriteError encodes with huma's JSON format, so an
// override of huma.NewError reads the same through it as through huma. encoding/json's
// default would escape the <, > and & huma writes as is.
func TestWriteErrorEncodesLikeHuma(t *testing.T) {
	orig := huma.NewError

	t.Cleanup(func() { huma.NewError = orig })

	huma.NewError = func(status int, _ string, errs ...error) huma.StatusError {
		return orig(status, "a<b&c", errs...)
	}

	res := httptest.NewRecorder()

	middleware.WriteError(res, http.StatusNotFound)

	if want := `"detail":"a<b&c"`; !strings.Contains(res.Body.String(), want) {
		t.Fatalf("body = %q, want it to contain %s", res.Body.String(), want)
	}
}
