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

Because of these gaps, call sites build requests by hand and call `httpClient.Do` directly,
and each one re-implements URL construction and error handling. There are about 46 such
non-test sites across 27 files today (excluding the generated
`internal/barista/observability/telemetry.twirp.go`). The in-flight `api.Client` migration
(tracking issue #13991) is converting the mechanical ones; **eight are projected to remain
once it lands**, because they need one of the three capabilities above:

`repoExists`, `downloadAsset` (extension), `fetchCommitSHA`, `publishedReleaseExists`,
`editRelease`, `downloadArtifact`, `apiLogFetcher.GetLog`, `getLog`.

Everywhere below, "the eight deferred sites" means that projected residue, not the current
count. None of the #13991 work is on `trunk` yet.

Separately, `api.Client` constructs a brand-new go-gh `RESTClient` on **every call**. The
reason is `Host`: go-gh bakes the host into a `RESTClient` at construction, while
`api.Client.REST(hostname, ...)` takes it per call. The two headers in `clientOptions` are
constants (`Authorization: ""` and the API-version header) and `SkipDefaultHeaders` is true,
so this is a workaround for per-call **host**, not for headers.

This matters for the design: `githubv3.Client` also holds a host, so `api.Client.REST` will
still construct a client per call. That is acceptable because the cost changes from
rebuilding go-gh's layered transport chain to allocating a three-field struct, but the
per-call construction does not disappear and the spec should not imply it does.

## Goals

- Provide a primitive that exposes the `*http.Request` and the `*http.Response`, so packages
  can make decisions a shared helper cannot make for them.
- Close all three gaps above without changing any existing behaviour.
- Keep `api.Client.REST` working unchanged for the majority of callers.
- Resolve API host and token once, at client construction, in a single place, so that
  API-host routing can later become configuration-driven without touching client code.

## Non-goals

- **GraphQL.** `api.Client.GraphQL`, `Query`, `Mutate` and `QueryWithContext` are unaffected
  and stay where they are. This package is REST-only by construction.
- **Rate-limit tracking.** google/go-github's `RateLimitError` and `AbuseRateLimitError`
  have no equivalent here.
- **Migrating `gh api`.** It is a deliberate passthrough: arbitrary methods, arbitrary
  multi-value headers, and a hand-set `ContentLength`. It stays on the raw `*http.Client`.
- **Consolidating scopes-suggestion logic with `project/shared/queries`.** That copy
  (`handleError`, `requiredScopesFromServerMessage`) matches on `api.GraphQLError`, filters
  `e.Type == "INSUFFICIENT_SCOPES"`, and recovers scopes by regexing the **server's message
  text**. It never reads `X-Accepted-Oauth-Scopes` or `X-Oauth-Scopes`, so a REST-only
  `ErrorResponse.ScopesSuggestion()` cannot serve it. This consolidation has also been tried
  and rejected once already: `fdd638808` ("Fixes #12823") was reverted by `d45acae60`. The
  only thing genuinely shared is the `gh auth refresh -s ...` wording.
- **Rewriting call sites wholesale.** Migration is incremental and driven by need.

## Design

### Package

`internal/githubv3`.

The name states that this is GitHub's REST API, which is v3, as distinct from GraphQL, which
is v4. It pairs with `github.com/shurcooL/githubv4`, which is imported in 51 files. Because
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
It also resolves an existing inconsistency in URL derivation. `ghinstance.RESTPrefix` and
go-gh's own `restPrefix` disagree: go-gh normalizes the hostname via
`auth.NormalizeHostname` before building the URL and `ghinstance.RESTPrefix` does not, so
`subdomain.github.com` yields `https://api.subdomain.github.com/` from one and
`https://api.github.com/` from the other. Which URL a request gets therefore depends on which
client built it. Resolving once, in the factory, removes the fork.

(This is *not* a disagreement with `AddAuthTokenHeader`. That normalizes for **token lookup**,
not URL construction, so it cannot conflict with a prefix function; it resolves the right
token either way.)

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
hostname from the request (via `getHost`, which prefers `req.Host` and falls back to
`req.URL.Host`) and looks up that host's token, because `api.NewHTTPClient`
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

A per-request option beats a default header, but not because of ordering inside `NewRequest`.
`NewRequest` does not set the defaults at all: `User-Agent`, `X-GitHub-Api-Version` and, when
`SkipDefaultHeaders` is false, `Accept`/`Content-Type`/`Time-Zone` are applied by go-gh's
`headerRoundTripper` at RoundTrip time, and it **skips any header already present on the
request**. So an option wins by virtue of being set earlier, not later. Stating this precisely
matters: an implementer who assumes a defaults-then-options pass inside `NewRequest` will
write one and produce double-set headers.

Because a `RequestOption` receives the `*http.Request` itself, it is not limited to
single-value headers, and callers who need something no option covers can mutate the request
between `NewRequest` and `Send`.

#### Relationship to `safeurl`

`path` is a `string`, matching `http.NewRequest`. `githubv3` does not depend on
`internal/safeurl`.

This is deliberate. The SafeURL guarantee is enforced at **domain helper signatures**, not at
the HTTP boundary: `downloadAsset(httpClient, assetURL safeurl.SafeURL, destPath string)`
constrains its untrusted input, then calls `assetURL.String()` one line before building the
request, because `http.NewRequest` takes a string. `githubv3.NewRequest` sits in exactly that
position, so taking the rendered string preserves the existing guarantee rather than weakening
it.

Requiring `safeurl.SafeURL` here would be worse than it looks. The two highest-traffic callers
cannot honour it: `api.Client.REST` keeps `p string` across 96 non-test call sites, and
`gh api` accepts arbitrary user-supplied paths. Both would have to call
`safeurl.NewImmutableSafeURL`, which bypasses all encoding by design, making the documented
escape hatch the dominant path. A guarantee the main road forges reads as type-system
enforcement while enforcing nothing.

A thin SafeURL wrapper package was considered and rejected: its body would be
`return c.NewRequest(ctx, method, u.String(), body, opts...)`, it could not be made mandatory,
and it would add a second way to do the same thing.

The protection is therefore a migration rule, reviewable per diff:

> **Migrating a call site to `githubv3` must not weaken an existing hardened parameter type.**
> An existing `safeurl.SafeURL` (or `safepaths.Absolute`) parameter stays as it is; the
> boundary lives on the domain helper, and `githubv3.NewRequest` receives the rendered string,
> exactly as `http.NewRequest` does today.

This matters for four of the eight deferred sites, which already take a `SafeURL`:
`downloadAsset` (both the extension and `release/download` copies), `downloadArtifact` and
`getLog`. `downloadArtifact` additionally takes a `safepaths.Absolute` destination.

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

`NextPage` returns a `string`, matching `RESTWithNext`'s current return type. Eleven call
sites already do `pageURL = safeurl.NewImmutableSafeURL(next)`; returning a string leaves them
untouched by migration, and is consistent with the `safeurl` boundary rule above.

**`Send` must close the response body on the error path.** It is documented as leaving the
body open for the caller, but a caller that receives `(nil, *ErrorResponse)` has nothing to
close. Note that go-gh's `HandleHTTPError` reads the body without closing it, so the close is
`Send`'s responsibility. `HandleHTTPError` also dereferences `resp.Request.URL` unguarded.

`Do` with a nil `v` discards the body without decoding. This is a **behaviour change**:
`api.Client.REST(..., nil)` today still runs `json.Unmarshal` into a nil interface, so a 200
with an empty body currently returns an "unexpected end of JSON input" error and afterwards
will not. Discarding is the better behaviour, but it must be called out rather than described
as behaviour-preserving.

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

That also lets the two REST copies of this logic collapse onto one: the method and
`api.ScopesSuggestion(resp)`. The partial reimplementation in `pkg/cmd/project/shared/queries`
is explicitly out of scope, for the reasons given under Non-goals.

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
| `restURL` / `restPrefix` (~15 lines) | no, the factory's resolver replaces it, including the `isGarage` and `github.localhost` special cases |
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
3. Reimplement `api.Client.REST` and `RESTWithNext` on top of it. Two behaviours must be
   carried over deliberately rather than assumed:
   - go-gh's `DoWithContext` skips decoding on **204 and 205** (`StatusNoContent` and
     `StatusResetContent`). `RESTWithNext` handles only 204. Reimplementing both on one
     `githubv3.Do` must preserve each method's current behaviour, not unify them by accident.
   - Owning URL derivation means replicating go-gh's `restPrefix` special cases: `isGarage`,
     `auth.IsEnterprise`, and `github.localhost`, which yields **`http://`**, not `https://`.
     Because go-gh normalizes and `ghinstance.RESTPrefix` does not, picking either one changes
     the URL for `subdomain.github.com` relative to the other, so this step must state which
     rule it adopts and which call sites that moves.
4. Replace `api.HTTPError` with `githubv3.ErrorResponse`. There are 115 non-test references
   across 33 non-test files (181 across 53 including tests). Each declaration changes type,
   and because the new type is used as a pointer where the current one is a value, `errors.As`
   sites also gain a `*`. Field reads such as `.StatusCode` and `.Message` are unchanged, and
   the two `ScopesSuggestion()` call sites keep working because the method is retained. Every
   change is compiler-caught, with two exceptions that are not:

   **(a) This step must fix `internal/ghcmd/cmd.go`.** Line 243 calls
   `httpErr.ScopesSuggestion()` in an `else if` that is reached when `errors.As` returned
   **false**. That is harmless today only because `api.HTTPError` is a value type with a value
   receiver returning a stored string, so the zero value yields `""`. After this step
   `httpErr` is a `*githubv3.ErrorResponse`, nil on a failed `errors.As`, and the method is
   computed, so it dereferences `e.StatusCode` and `e.Headers` and panics. This fires on the
   common path of any non-API error reaching top-level handling. `authRecoveryCommand(cfg
   gh.Config, httpErr api.HTTPError)` takes a value too, so its signature changes as well.
   Both the pointer change and the computed-method change are individually safe; together
   they are a crash, so restructuring that `else if` chain is required work in this step and
   not a pre-existing issue to defer.

   **(b) `pkg/httpmock/stub.go` imports `api`** for
   `JSONErrorResponse(status int, err api.HTTPError)`. After this step it imports `githubv3`
   instead, so `githubv3`'s own in-package tests cannot use `httpmock`. Use an external
   `package githubv3_test` for any test that needs it.
5. Migrate the eight deferred `httpClient.Do` sites, which are the ones that need the new
   surface: `repoExists`, `downloadAsset`, `fetchCommitSHA`, `publishedReleaseExists`,
   `editRelease`, `downloadArtifact`, `apiLogFetcher.GetLog` and `getLog`. Two of these treat
   404 as a `false` rather than an error (`repoExists`, `publishedReleaseExists`); under
   `Send` they must use `errors.As` on `*ErrorResponse` and check `StatusCode`, since a
   non-2xx now arrives as an error. Observe the `safeurl` migration rule above.
6. Convert `gh auth status` to construct one client per host inside its existing host loop,
   sharing the underlying `*http.Client`. Today it uses a single host-agnostic client across
   every configured host and every user within a host, which works only because
   `auth/shared/oauth_scopes.go` sets `Authorization` itself. Making the per-host client
   explicit removes the reliance on that generic behaviour.
7. Opportunistically move the remaining raw `httpClient.Do` sites off the raw client. This is
   the residue of #13991 plus the custom-header sites, not a small set: see the count under
   Problem.

## Testing

Unchanged in mechanism. `httpmock.Registry` is an `http.RoundTripper` and is injected through
the `*http.Client`, exactly as it is for `api.Client` today. Because `githubv3` takes a
resolved base URL and token, its own tests need neither configuration nor a factory.

New tests for `githubv3` cover: relative and absolute paths; `Authorization` set from the
client token, omitted when the token is empty, and overridden by `WithToken`; a
`RequestOption` header surviving rather than being replaced by go-gh's `headerRoundTripper`,
which is the mechanism that gives options precedence; `Send` leaving the body open and `Do`
closing it, including `Send` closing it on the error path; `Do` with a nil `v` discarding the
body; 204 and 205 skipping decode; 2xx status reaching the caller via `Response.StatusCode`;
non-2xx producing an `*ErrorResponse` with the decoded `message` and `errors`;
`ScopesSuggestion` across the 4xx range including 401 and excluding 422; and `NextPage` with a
`Link` header present, absent, and containing multiple relations.

`githubv3`'s tests that need `httpmock` must live in an external `package githubv3_test`, for
the import-cycle reason given in step 4(b).

Step 4 needs a regression test that a non-API error reaching `internal/ghcmd`'s error handling
does not panic, since that is the failure mode 4(a) introduces and the compiler cannot catch
it.

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
a suggestion never prints it.

That last branch also calls a method on `httpErr` when `errors.As` returned false, and stays
quiet only because the current type is a value whose stored field is empty. **That half stops
being a latent issue and becomes a nil-pointer panic under step 4, so step 4 must fix it.**
See step 4(a); it is recorded here only because the 401 short-circuit is a genuinely separate
bug in the same block.

Fixing the 401 short-circuit itself is out of scope, but making `ScopesSuggestion` a computed
method is a prerequisite for it. It should be tracked separately.

### Parallel authenticated-request implementation in `oauth_scopes.go`

`pkg/cmd/auth/shared/oauth_scopes.go` hand-builds a request, sets `Authorization` itself, and
sends it purely to read the `X-Oauth-Scopes` response header. It is a second implementation of
"make an authenticated GitHub request", independent of `api.Client`. It works, and it is what
currently allows `gh auth status` to check several users' tokens through one host-agnostic
client.

Once `githubv3` exists it reduces to constructing a per-host client and calling `Send`, but
converting it is a follow-up rather than part of this work.

