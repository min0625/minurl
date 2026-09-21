// Copyright 2026 The MinURL Authors

package middleware

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// RequestDecompress returns a huma middleware that decompresses a request body sent with
// Content-Encoding: gzip. It runs inside the API, not on the router, for two reasons: its
// errors are api's ErrorModel, with the statuses huma itself returns while reading a body,
// and it can skip an operation without a body, whose Content-Encoding huma ignores along
// with the body itself.
//
// Any other encoding is a 415. A gzip body that is corrupt is a 400, and one that stalls past
// the operation's body read timeout a 408. The operation's MaxBodyBytes applies to the body
// both as sent and decompressed, as huma applies it to a plain body: reaching it as sent is a
// 413 here, and reaching it decompressed is huma's own 413, before the body expands further.
// So gzip never raises the limit, though for a body that does not compress it lowers it by
// gzip's overhead.
func RequestDecompress(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		op := ctx.Operation()

		encoding := strings.TrimSpace(ctx.Header("Content-Encoding"))
		if encoding == "" || op.RequestBody == nil {
			next(ctx)
			return
		}

		// huma sets the body read deadline once its middlewares have run, too late for the
		// read below, so set it here the same way. The 415 needs it too: before writing a
		// response, net/http reads what is left of a small body, and would wait for a stalled
		// one until the server's ReadTimeout, by when its WriteTimeout has passed as well.
		// https://github.com/danielgtaylor/huma/blob/v2.37.3/huma.go#L908-L914
		if op.BodyReadTimeout > 0 {
			_ = ctx.SetReadDeadline(time.Now().Add(op.BodyReadTimeout))
		} else if op.BodyReadTimeout < 0 {
			_ = ctx.SetReadDeadline(time.Time{})
		}

		if !strings.EqualFold(encoding, "gzip") {
			_ = huma.WriteErr(api, ctx, http.StatusUnsupportedMediaType,
				"unsupported Content-Encoding, only gzip is accepted")

			return
		}

		data, err := readGzip(ctx.BodyReader(), op.MaxBodyBytes)
		netErr, _ := errors.AsType[net.Error](err)
		_, tooLarge := errors.AsType[*http.MaxBytesError](err)

		switch {
		case netErr != nil && netErr.Timeout():
			_ = huma.WriteErr(api, ctx, http.StatusRequestTimeout, "request body read timeout")
		case tooLarge:
			_ = huma.WriteErr(api, ctx, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("request body is too large limit=%d bytes", op.MaxBodyBytes))
		case err != nil:
			_ = huma.WriteErr(api, ctx, http.StatusBadRequest, "invalid gzip request body")
		default:
			next(decompressedContext{humaContext: ctx, body: bytes.NewReader(data)})
		}
	}
}

// readGzip decompresses a gzip body. A positive maxBytes, as huma treats it, refuses a body
// that reaches it as sent, since a gzip member can take bytes without producing any, and stops
// the read once it decompresses to maxBytes, where huma answers with its own 413.
func readGzip(body io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes > 0 {
		// No ResponseWriter to flag: only the *http.MaxBytesError is wanted. One byte under
		// maxBytes, since MaxBytesReader accepts a body of exactly its limit and huma does not.
		// https://github.com/danielgtaylor/huma/blob/v2.37.3/huma.go#L2122-L2130
		body = http.MaxBytesReader(nil, io.NopCloser(body), maxBytes-1)
	}

	// Not closed: Close only returns an error the read has already returned.
	gr, err := gzip.NewReader(body)
	if err != nil {
		return nil, err
	}

	var r io.Reader = gr
	if maxBytes > 0 {
		r = io.LimitReader(gr, maxBytes)
	}

	return io.ReadAll(r)
}

// humaContext renames huma.Context for embedding: a field named Context would hide its
// Context method.
type humaContext huma.Context

// decompressedContext is handed downstream once RequestDecompress has read the whole body.
// It covers what huma reads a JSON body through, not a multipart form: humachi parses that
// from the raw request, which the middleware has already drained, so a gzip multipart
// request would fail with a 422. No operation takes a multipart form.
type decompressedContext struct {
	humaContext

	body io.Reader
}

var _ interface{ Unwrap() huma.Context } = decompressedContext{}

// Unwrap returns the context it wraps, as huma's own wrapper does, so humachi.Unwrap reaches
// the request through it.
func (c decompressedContext) Unwrap() huma.Context {
	return c.humaContext
}

// BodyReader returns the decompressed body.
func (c decompressedContext) BodyReader() io.Reader {
	return c.body
}

// SetReadDeadline ignores the deadline huma sets before reading the body, which is already
// read. When a body reaches EOF, net/http starts a background read on the connection, and a
// read deadline set after that cancels the request context when it fires, so a gzip request
// still in its handler at the body read timeout would fail with context.Canceled.
//
//   - EOF starts the background read: https://github.com/golang/go/blob/go1.26.8/src/net/http/server.go#L2053-L2057
//   - which clears the deadline first: https://github.com/golang/go/blob/go1.26.8/src/net/http/server.go#L687-L699
//   - a timeout on it cancels the context: https://github.com/golang/go/blob/go1.26.8/src/net/http/server.go#L729-L733
//     and https://github.com/golang/go/blob/go1.26.8/src/net/http/server.go#L769-L777
func (decompressedContext) SetReadDeadline(time.Time) error {
	return nil
}
