# HTTP API design

How requests are validated and how every error reaches the client. Read this before changing
`internal/handler`, `internal/middleware`, `internal/httpserver`, a request or response type, a
validation rule, or an error status. The procedures that follow from it are in
[CONTRIBUTING.md](../../CONTRIBUTING.md).

## Request validation

**Validation lives in the request schema, not in a second validator.** huma checks the
schema against the parsed JSON *before* it unmarshals into the struct, so it can tell an
absent key from a zero value, its errors carry `location` and `value`, and the rules reach
the generated OpenAPI. A struct-level validator sees none of that.

**A rule that belongs to a field belongs to its type.** `service.OriginalURL` is the
worked example: `huma.SchemaProvider` gives it `minLength`, and `huma.ResolverWithPath`
gives it the scheme/host/userinfo checks and the whitespace/control/invisible-character
screen JSON Schema cannot express.
huma calls the resolver for every field of that type and supplies the error location
itself — no hand-written `"body.original_url"` to go stale.

Two things such a resolver must respect: a value-typed field reaches it as the zero value
when the key is absent, so return early on empty rather than inventing an error for a
field nobody sent (a pointer field is skipped instead); and it runs on request input only,
which is why `ShortURLService.Get` calls `IsValidOriginalURL` itself for stored rows.

**`huma.ValidateStrictCasing` is load-bearing.** huma validates the exact JSON key, but
`encoding/json` then matches keys case-insensitively and keeps the last one, so without it
`{"id":"abc","ID":"bad*id"}` stores the unvalidated `ID`, and `"ORIGINAL_URL":""` slips an
empty URL past `minLength`. It is set in `init()` in `internal/handler/register.go`
(not in `Register`: tests build APIs in parallel, so a write there would be a data race).

`OriginalURL` has **no length limit, deliberately**. `minLength` is load-bearing though:
`required:"true"` only asserts the key is present, and the resolver returns nil for the
empty string, so dropping `minLength` lets an empty `original_url` through.

**An optional field's schema must accept the Go zero value.** `ShortURL.ID` uses `*`
rather than `+` in its pattern: `Create` already treats an empty ID as "generate one", and
a Go client without `omitzero` serializes an unset string as `""`, so `+` would 422 a
request that means the same as an omitted one. The `{id}` path param keeps `+` — an empty
path segment never reaches the pattern, because huma rejects it first with "required path
parameter is missing".

`ShortURL.ID` is deliberately **not** `nullable`. `ShortURL` is both the request body and
the response, so one schema describes both, and a nullable `id` would tell clients a
response may carry `"id": null`, which the server never sends. Input stays lenient anyway:
huma skips `null` for any non-required property whatever its schema says, so
`{"id": null}` still means "generate one" — the document just does not advertise it.
Declaring `null` for input only would take separate request and response types.

The `*`/`+` difference is why `ShortURL.ID` stays on tags instead of becoming a named type like
`OriginalURL`: one `Schema()` returns one schema, but the body needs `*` and the path
needs `+`. There are exactly two declarations: the body field, and one `shortURLIDInput` that the
get and redirect operations share because their parameter is identical. `TestRegisterPublishesShortIDConstraints`
pins the body property and both published path params to `Base58Alphabet` and
`MaxShortURLIDLen`.

## Error responses

Three pieces in `internal/handler/register.go`:

1. **The service defines the errors** (`service.ErrShortURLNotFound`, `ErrShortURLIDConflict`, …).
2. **`errorResponses` gives each error its status and message, once**, for every operation.
3. **Every operation is registered through `register(api, operation{…}, handler)`**, never
   `huma.Register` directly. `operation.errs` lists the errors the operation returns; `register`
   publishes their statuses as `Operation.Errors` and converts the handler's errors through the
   same list. The handler returns service errors as they are.

**`register` takes its own `operation`, not a `huma.Operation`.** It builds the `huma.Operation`
itself, so fields that change which statuses huma returns cannot be set around it: `Errors` /
`Responses`, `MaxBodyBytes` / `BodyReadTimeout` (a negative value disables the 413 / 408 it
publishes), `SkipValidateBody` / `SkipValidateParams` / `RejectUnknownQueryParameters` (remove or
add a 422), `Middlewares` (write any status without passing through `toHTTPError`), `RequestBody`
and `Hidden`. Add a field to `operation` only once it is clear it changes none of the published
statuses, and copy it in `register` (`TestRegisterBuildsTheHumaOperation` pins that copy). Tags
are not a field yet: every operation is tagged `ShortURL`; add one with the first operation that
needs another tag.

A handler cannot return an error status the OpenAPI document does not list: every error it
returns goes through `toHTTPError`, so even a `huma.ErrorXXX` it builds itself is not in `errs`
and becomes a 500. Only `toHTTPError` returns a `huma.ErrorXXX` / `huma.NewError`. The success
status is `operation.defaultStatus`; outputs have no `Status` field, which huma would write as-is.
The steps to add a failure are in
[CONTRIBUTING.md — Adding an Error Status](../../CONTRIBUTING.md#adding-an-error-status).

- **An error the operation does not list is a 500**, even if `errorResponses` knows it: its real
  status is unpublished for that operation. Its log line carries `unlisted_error=true`.
- **Any 500 a handler returns carries no detail and is logged instead**, with the request's
  `method`, `path`, `request_id` and trace attributes. huma serializes an attached error into the
  body, and the store wraps driver errors that name tables and columns. A `context.Canceled` (the
  client went away) is logged at WARN, not ERROR, but is still a 500 in the access log and span.
  huma's own 500 for a body the client cut short never reaches `toHTTPError`: it carries the read
  error (`unexpected EOF`), which names nothing on the server side.
- **`register` panics at registration rather than publish a list it cannot vouch for**, so every
  test that builds an API fails: an error `errorResponses` lacks, and an input with a `RawBody`
  field (see below).
- **The published list must be exhaustive for what the server returns.** A non-empty
  `Operation.Errors` makes huma skip its `default` response; `register` adds `default` once
  `huma.Register` returns, to the `Responses` map it handed in — huma publishes that same map —
  so the Kiota client still decodes an `ErrorModel` for a status nobody lists (a gateway's 502, a
  status a huma upgrade adds). Its content is the map huma wrote for `500`, the status every
  operation lists, shared rather than copied. `default` is that safety net, not a substitute for
  listing.
- **Statuses returned before the handler runs** (`bodyReadErrors`: 400 / 408 / 413 / 415) cannot
  come from a service error. huma returns them while reading the body, and
  `middleware.RequestDecompress` returns them for a body with a `Content-Encoding` (400 / 408 /
  413 for gzip, 415 for any other encoding), both as `ErrorModel`.
  `register` adds them when the input has a `Body` field (promoted fields count, as they do for
  huma), since that is what makes huma limit, read and parse the body. A `RawBody` gets no size
  limit and is never parsed (no 413 / 415), so it panics until someone works out its statuses and
  adds reachability cases. `register` always adds 500, which keeps `Operation.Errors` non-empty so
  huma appends 422 and 500 itself.
- **The lists cannot prove the service still returns each error.** The reachability test covers
  that direction, so every listed *error*, not just every status, needs a request that returns
  it — `ErrOriginalURLTooLong` and the body limit are both 413. It drives `httpserver.BuildAPI`,
  so the middleware's statuses count, and reads the document `BuildOpenAPISpec` publishes.
- **HTTP status stays out of `service`.** Do not implement `huma.StatusError` on service errors:
  huma would use that status directly, bypassing the operation's list and the document.
- **Resolvers are outside the funnel.** huma answers with the status of any `StatusError` a
  resolver returns, never seeing `toHTTPError`. A resolver returns `*huma.ErrorDetail` (always
  422, as `OriginalURL.Resolve` does), never `huma.ErrorXXX`.
- **Responses huma never sees still answer an `ErrorModel`, through `middleware.WriteError`**:
  a recovered panic (`PanicRecovery`), and a request no operation matches — `NewRouter` sets
  chi's `NotFound` and `MethodNotAllowed`. chi writes `Allow` only in its own 405 handler, so the
  custom one rebuilds it with `Mux.Match`, and answers 404 when no method matches: chi sends a
  method it does not know there before it looks the path up. The body comes from
  `huma.NewError`, like `toHTTPError`'s 500, so an override of it reaches these too. Only what
  `net/http` refuses before routing (e.g. a 431, or a 400 for a malformed request line) gets no
  `ErrorModel`.

## Expiry and stored rows

Expiry is enforced in `ShortURLService.Get()` (`internal/service/short_url.go`), not in the store,
which returns raw rows. So is a stored `original_url` that breaks `IsValidOriginalURL` (a row older than the create rules):
`Get` returns `ErrShortURLNotFound` for it, as for an expired row, and logs a WARN with the `id`.
