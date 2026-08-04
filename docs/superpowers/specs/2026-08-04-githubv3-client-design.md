# `githubv3`: a lower-level GitHub REST API client

## Problem

`api.Client.REST` decodes a JSON body into a caller-supplied value and returns only an
`error`. That surface cannot express three things the CLI needs:

1. **Per-request headers.** Around thirteen call sites set a custom `Accept` header, spread
   across `pr/diff` (diff and patch media types), `repo/read-file` (`raw`, `object+json`),
   `repo/edit` (`mercy-preview`), `search` (`text-match+json`), `release/download` and
   `release/create` (`octet-stream`), `extension` (`octet-stream`, `v3.sha`) and
   `codespaces`. `gh api` additionally forwards arbitrary user-supplied headers with
   `Header.Add`, so multi-value headers are a real requirement even though `gh api` itself
   stays on the raw client; this is what rules out a `map[string]string` option shape.
2. **Raw and streaming response bodies.** Four call sites stream a response to disk or hand
   it to a caller as an `io.ReadCloser` rather than decoding it.
3. **Status codes on success.** Three call sites branch on which 2xx they received, which
   `REST` discards.

Because of these gaps, roughly eight call sites still build requests by hand and call
`httpClient.Do` directly, and each one re-implements URL construction and error handling.
The in-flight `api.Client` migration (tracking issue #13991) deferred all of them.

Separately, `api.Client` constructs a brand-new go-gh `RESTClient` on **every call**, purely
so it can pass `ClientOptions.Headers`. That is a workaround for gap 1, not a design.

## Goals

- Provide a primitive that exposes the `*http.Request` and the `*http.Response`, so packages
  can make decisions a shared helper cannot make for them.
- Close all three gaps above without changing any existing behaviour.
- Keep `api.Client.REST` working unchanged for the majority of callers.
- Consolidate the three copies of the OAuth-scopes-suggestion logic onto one implementation.
- Resolve API host and token once, at client construction, in a single place, so that
  API-host routing can later become configuration-driven without touching client code.

## Non-goals

- **GraphQL.** `api.Client.GraphQL`, `Query`, `Mutate` and `QueryWithContext` are unaffected
  and stay where they are. This package is REST-only by construction.
- **Rate-limit tracking.** google/go-github's `RateLimitError` and `AbuseRateLimitError`
  have no equivalent here.
- **Migrating `gh api`.** It is a deliberate passthrough: arbitrary methods, arbitrary
  multi-value headers, and a hand-set `ContentLength`. It stays on the raw `*http.Client`.
- **Rewriting call sites wholesale.** Migration is incremental and driven by need.

## Design

### Package

`internal/githubv3`.

The name states that this is GitHub's REST API, which is v3, as distinct from GraphQL, which
is v4. It pairs with `github.com/shurcooL/githubv4`, already imported in four files. Because
that adjacency could imply a shared origin that does not exist, the package doc says so
explicitly:

```go
// Package githubv3 is a client for GitHub's REST (v3) API. It is unrelated to
// github.com/shurcooL/githubv4, which is a GraphQL query DSL rather than a client.
package githubv3
```

### Client

```go
type Client struct {
    apiBaseURL *url.URL
    token      string
    http       *http.Client
}

func NewClient(apiBaseURL *url.URL, token string, httpClient *http.Client) *Client
```

The client holds an **already-resolved** API base URL and token. It does not import
`ghinstance`, does not read configuration, and has no opinion about how a canonical host such
as `github.example.com` becomes an API host. Resolution happens once, at construction, in the
factory:

```go
func (f *Factory) GitHubClient(host string) (*githubv3.Client, error)
```

which reads config, maps canonical host to API host, fetches the token, and returns a ready
client. It must be called lazily inside `RunE`, since `BaseRepo` is itself lazy.

This keeps the config-driven parts in one place. When API-host routing becomes
configuration-driven rather than the rule-based `ghinstance.RESTPrefix`, the change lands
entirely in the factory's resolver; `githubv3` is unaffected and its tests need only a URL.
It also resolves an existing inconsistency in which `AddAuthTokenHeader` normalizes the
hostname via `ghauth.NormalizeHostname` but `ghinstance.RESTPrefix` does not, so the two
disagree for inputs such as `GITHUB.COM` and `subdomain.github.com`.

The factory has everything it needs except the host itself, which stays a parameter exactly as
it is for `api.Client.REST(hostname, ...)` today. Hostnames come from `RepoHost()` (282 call
sites), `DefaultHost()` (69), `ghinstance.Default()` (5) and `--hostname` flags (10); the
first and last are known to the caller, the rest come from config.

A token is passed as a value rather than a getter. It cannot change during a client's
lifetime, since configuration is read once. An empty token means an unauthenticated request.

`CAPIClient` already pairs an `*http.Client` with a `host` field, so a client that carries its
own host is an established shape in the codebase.

#### Relationship to `AddAuthTokenHeader`

Today the token is attached by `AddAuthTokenHeader`, a `RoundTripper` that derives the
hostname from the request URL and looks up that host's token, because `api.NewHTTPClient`
deliberately builds the underlying client with `Host: "none"` and `AuthToken: "none"` so go-gh
will not resolve them. That indirection exists to support a host-agnostic `*http.Client` that
is passed around generically.

`githubv3` sets `Authorization` itself, and the two coexist safely without removing the
transport:

- The transport skips when the header is already set, so it never overwrites ours.
- On a cross-host redirect, Go's `http.Client` strips `Authorization` before the transport
  runs. This was verified empirically: a same-host redirect preserves the header, a
  cross-host redirect drops it. That is the same protection the transport's own
  `redirectHostnameChange` branch provides by hand, so the guarantee does not depend on which
  mechanism is in play.
- On such a redirect the transport then looks up a token for the new host, for example a CDN
  host serving a release asset, and finds none.

Removing `AddAuthTokenHeader` is therefore not required by this design and is out of scope.

### Requests

```go
type RequestOption func(*http.Request)

func WithHeader(name, value string) RequestOption
func WithToken(token string) RequestOption

func (c *Client) NewRequest(ctx context.Context, method, path string, body io.Reader, opts ...RequestOption) (*http.Request, error)
```

`path` may be a path relative to the client's API base URL, or an absolute URL. Absolute URLs
are required by call sites that follow server-supplied URLs, such as release asset uploads.

`NewRequest` sets `Authorization` from the client's token and then applies options, so an
option can override it. This makes token overrides explicit at the call site rather than
implicit in the transport's "do not overwrite" rule. `gh auth status` and
`auth/shared/login_flow.go` need this: they check a *specific* user's token rather than the
host's active one.

Options are applied **after** the client's own defaults, so a per-request header overrides a
default. Because a `RequestOption` receives the `*http.Request` itself, it is not limited to
single-value headers, and callers who need something no option covers can mutate the request
between `NewRequest` and `Send`.

Context is supplied to `NewRequest` and travels on the request, so `Send` and `Do` take no
`ctx` parameter. This matches `http.NewRequestWithContext` and `http.Client.Do`.

### Responses

```go
type Response struct {
    *http.Response
}

// NextPage returns the rel="next" URL from the Link header, or "" if absent.
func (r *Response) NextPage() string

// Send issues req and returns the response with its body still open.
// The caller must close it. A non-2xx status yields *ErrorResponse.
func (c *Client) Send(req *http.Request) (*Response, error)

// Do issues req, decodes the body into v, and closes it.
// A nil v discards the body without decoding.
func (c *Client) Do(req *http.Request, v any) (*Response, error)
```

Two methods rather than one, because `apiLogFetcher.GetLog` returns the response body to its
caller as an `io.ReadCloser`. A single always-closing method cannot serve it. This is the same
reason google/go-github separates `BareDo` from `Do`.

`NextPage` absorbs the `Link` header parsing currently in `api.findNextPage`, so
`RESTWithNext` reduces to a `Do` plus a `NextPage`. Keeping the opaque next URL rather than
go-github's parsed page *numbers* means cursor-based pagination keeps working.

### Errors

```go
type ErrorResponse struct {
    Errors     []ErrorItem
    Headers    http.Header
    Message    string
    RequestURL *url.URL
    StatusCode int
}

func (e *ErrorResponse) Error() string
func (e *ErrorResponse) ScopesSuggestion() string
```

`ErrorItem` mirrors go-gh's `HTTPErrorItem`, so the decoded payload keeps its current shape.

This replaces `api.HTTPError`. Three things change.

**The name.** `HTTPError` describes the layer, when the useful distinction is the source. This
type is only ever constructed when the API *successfully responded* and reported failure in
its own payload, carrying GitHub's `message` and `errors` fields. A DNS failure or connection
reset is a genuine HTTP error and is never this type.

**`ScopesSuggestion` is computed, not stored.** `api.HTTPError` holds a private
`scopesSuggestion` field populated at construction from `StatusCode`, two headers and
`RequestURL.Hostname()`. Every one of those inputs is already a field on the error, so the
field is a cached copy of a pure function of the value that holds it. Computing on demand
removes the duplicate state and means a suggestion is always available when one exists.

That also lets the three current copies of this logic collapse onto one: the method,
`api.ScopesSuggestion(resp)`, and the partial reimplementation in
`pkg/cmd/project/shared/queries`, which carries a `// TODO: this duplicates parts of
generateScopesSuggestion` comment.

**It does not carry an `*http.Response`.** google/go-github's `ErrorResponse` does, which
forces it to re-buffer the body so the response inside the error is still readable; its own
comment cites issues #1136 and #540. Nothing in this codebase needs the raw error body, and
the flattened fields include `Headers`, so nothing is lost. The name is a slight overstatement
of the shape: the type holds the response's *content*, destructured.

`Send` and `Do` return `(nil, *ErrorResponse)` on a non-2xx status, matching the existing
`errors.As` pattern at call sites.

### Relationship to go-gh

`githubv3` keeps go-gh for everything substantial and replaces only its thin request wrapper:

| go-gh component | used by `githubv3` |
| --- | --- |
| transport, caching, logging | yes |
| `ClientOptions` | yes |
| `HandleHTTPError` (error payload parsing) | yes |
| `restURL` / `restPrefix` (~15 lines) | no, the factory's resolver replaces it |
| `RESTClient.RequestWithContext` (~20 lines) | no, replaced |

`RESTClient` is not used, because it cannot support this design. Its only header mechanism is
`ClientOptions.Headers map[string]string`, fixed at client construction: single-valued, and
requiring a fresh client per call. It builds the `*http.Request` internally and never returns
it, so a `RequestOption func(*http.Request)` is impossible, and `ctx` must be threaded through
every method instead of riding on the request.

URL derivation moves to the factory's resolver rather than being duplicated per client, which
is what lets API-host routing become configuration-driven later. The normalization
inconsistency this resolves is described under Client above.

The gaps this works around are real for extension authors too, who have no `api.Client` to
hide behind. Proposing per-request options upstream to go-gh should be tracked separately.

## Migration

Each step is independently shippable and behaviour-preserving.

1. Add `internal/githubv3` with no callers.
2. Add `Factory.GitHubClient(host)`, which owns config lookup, canonical-host to API-host
   mapping, and token resolution.
3. Reimplement `api.Client.REST` and `RESTWithNext` on top of it. The current `RESTWithNext`
   contains a dead `if !success` branch, because go-gh's `RESTClient.Request` already returns
   `(nil, err)` on non-2xx and so a response only ever reaches that check on success; the
   reimplementation drops it.
4. Replace `api.HTTPError` with `githubv3.ErrorResponse`. There are 63 references to
   `api.HTTPError` across 30 production files. Each declaration changes type, and because
   go-gh returns a pointer where the current type is used as a value, `errors.As` sites also
   gain a `*`. Field reads such as `.StatusCode` and `.Message` are unchanged, and the two
   `ScopesSuggestion()` call sites keep working because the method is retained. Every change
   is compiler-caught.
5. Migrate the eight deferred `httpClient.Do` sites, which are the ones that need the new
   surface: `repoExists`, `downloadAsset`, `fetchCommitSHA`, `publishedReleaseExists`,
   `editRelease`, `downloadArtifact`, `apiLogFetcher.GetLog` and `getLog`.
6. Convert `gh auth status` to construct one client per host inside its existing host loop,
   sharing the underlying `*http.Client`. Today it uses a single host-agnostic client across
   every configured host and every user within a host, which works only because
   `auth/shared/oauth_scopes.go` sets `Authorization` itself. Making the per-host client
   explicit removes the reliance on that generic behaviour.
7. Opportunistically move the remaining custom-header sites off raw `httpClient.Do`.

## Testing

Unchanged in mechanism. `httpmock.Registry` is an `http.RoundTripper` and is injected through
the `*http.Client`, exactly as it is for `api.Client` today. Because `githubv3` takes a
resolved base URL and token, its own tests need neither configuration nor a factory.

New tests for `githubv3` cover: relative and absolute paths; `Authorization` set from the
client token, omitted when the token is empty, and overridden by `WithToken`; header defaults
being overridable by a `RequestOption`; `Send` leaving the body open and `Do` closing it; `Do`
with a nil `v` discarding the body; 2xx status reaching the caller via `Response.StatusCode`;
non-2xx producing an `*ErrorResponse` with the decoded `message` and `errors`;
`ScopesSuggestion` across the 4xx range including 401 and excluding 422; and `NextPage` with a
`Link` header present, absent, and containing multiple relations.

Factory tests cover canonical-host to API-host mapping for github.com, enterprise, tenancy,
garage and localhost, matching the existing `ghinstance.RESTPrefix` rules.

Existing `api` tests are the regression suite for step 3, and must pass unmodified.

## Known issues found while designing

### Scopes suggestion dropped on 401

`internal/ghcmd/cmd.go` renders a scopes suggestion only in the final branch of an
`else if` chain:

```go
if errors.As(err, &httpErr) && httpErr.StatusCode == 401 {
    // prints "Try authenticating with: gh auth login"
} else if u := factory.SSOURL(); u != "" {
    ...
} else if msg := httpErr.ScopesSuggestion(); msg != "" {
```

`generateScopesSuggestion` fires for any 4xx except 422, which includes 401. So a 401 that has
a suggestion never prints it. That last branch also calls a method on `httpErr` when
`errors.As` returned false, and stays quiet only because the zero value's field is empty.

This is pre-existing and out of scope here, but making `ScopesSuggestion` a computed method
is a prerequisite for fixing it. It should be tracked separately.

### Parallel authenticated-request implementation in `oauth_scopes.go`

`pkg/cmd/auth/shared/oauth_scopes.go` hand-builds a request, sets `Authorization` itself, and
sends it purely to read the `X-Oauth-Scopes` response header. It is a second implementation of
"make an authenticated GitHub request", independent of `api.Client`. It works, and it is what
currently allows `gh auth status` to check several users' tokens through one host-agnostic
client.

Once `githubv3` exists it reduces to constructing a per-host client and calling `Send`, but
converting it is a follow-up rather than part of this work.

