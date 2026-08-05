# `githubrest`: a lower-level GitHub REST API client

> **Status.** The package exists as `internal/githubrest` and is unmigrated: it has no
> production callers. This document has been reconciled with the code as built, so the Design
> section describes what is there rather than what was originally proposed. Where implementing
> the design changed it, the change is described in place and summarised under
> [Divergences from the original design](#divergences-from-the-original-design). Migration
> steps 2 to 7 are untouched and remain the plan.

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

This matters for the design: `githubrest.Client` also holds a host, so `api.Client.REST` will
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

`internal/githubrest`.

The name was originally `githubv3`, on the reasoning that REST is v3 as distinct from GraphQL
at v4, pairing with `github.com/shurcooL/githubv4`, which is imported in 51 files. That was
dropped during implementation. GitHub now versions the REST API **by date**, through the
`X-GitHub-Api-Version` header, so "v3" describes the shape of the URL paths rather than the
API a caller actually gets. Naming the package after a version number that no longer selects
anything would age badly, and the adjacency to `githubv4` implies a shared origin that does
not exist. `githubrest` says what the package is:

```go
// Package githubrest is a client for GitHub's REST API.
//
// The API is versioned "v3" in its URL paths, but the name is avoided here:
// GitHub now versions the REST API by date through the X-GitHub-Api-Version
// header, so "v3" describes the URL layout rather than the API a caller gets.
// The name also collides confusingly with github.com/shurcooL/githubv4, which
// is a GraphQL query DSL rather than a REST client.
package githubrest
```

### Client

```go
type Client struct {
    apiBaseURL        *url.URL
    token             o.Option[string]
    http              *http.Client
    credentialedHosts map[string]struct{}
}

type AuthStrategy func(*Client)

func WithToken(token string) AuthStrategy
func WithoutToken() AuthStrategy

type ClientOption func(*Client)

func WithCredentialedHost(host string) ClientOption
func WithCheckRedirect(fn func(*http.Request, []*http.Request) error) ClientOption

func NewClient(apiBaseURL string, httpClient *http.Client, auth AuthStrategy, opts ...ClientOption) (*Client, error)
```

`apiBaseURL` is a `string` rather than a `*url.URL`, and construction returns an `error`. A
`*url.URL` looks like the stronger type but does not carry the invariant that matters:
`url.Parse` succeeds on relative input, so `url.Parse("api/v3")` and even `url.Parse("")`
return a usable `*url.URL` with no error. Only an explicit `Scheme != "" && Host != ""` check
establishes absoluteness, and that check has to return an error from somewhere. Validating in
the constructor means a `Client` either resolves relative paths correctly or does not exist.
A caller that already holds a parsed URL passes `u.String()`; `ghinstance.RESTPrefix`, the
resolver this replaces, returns a string anyway.

#### Authentication is a required argument

`AuthStrategy` is a **positional parameter**, not an option, so a caller cannot construct a
client without saying whether it authenticates. This is the one decision that is wrong in
both directions: a client that should authenticate but does not gets an unhelpful 404 for
anything private, and one that should not but does sends a user's token somewhere it was
never meant to go. Neither failure looks like a missing option at the call site, so the
compiler asks the question instead.

The original design had a bare `token string` where empty meant "unauthenticated". That
conflates two different intentions. `gh auth status` and `auth/shared/login_flow.go` check a
*specific* user's token rather than the host's active one, and must send exactly what they
hold and get whatever answer it earns, including when what they hold is empty. So:

- `WithToken(t)` always sets `Authorization`, including for an empty `t`, which renders as
  `"token "`. A header set to `""` would not do: `api.AddAuthTokenHeader` and go-gh's header
  round tripper both stand down only when `Header.Get("Authorization")` is **non-empty**, so
  an empty value would let either inject the active user's token and the command would report
  on the wrong credentials.
- `WithoutToken()` sets no header at all.

`WithoutToken` states the client's intent, which is as far as the package's reach goes. It is
**not a guarantee of anonymity**: an `http.Client` can carry a transport that fills in
credentials on any request lacking them, and this package neither knows nor controls what
transport it was given. Making that guarantee real means changing `api/http_client.go`, which
is separate work.

The client holds an **already-resolved** API base URL and token. It does not import
`ghinstance`, does not read configuration, and has no opinion about how a canonical host such
as `github.example.com` becomes an API host. Resolution happens once, at construction, in the
factory:

```go
func (f *Factory) GitHubClient(host string) (*githubrest.Client, error)
```

which reads config, maps canonical host to API host, fetches the token, and returns a ready
client. It must be called lazily inside `RunE`, since `BaseRepo` is itself lazy.

This keeps the config-driven parts in one place. When API-host routing becomes
configuration-driven rather than the rule-based `ghinstance.RESTPrefix`, the change lands
entirely in the factory's resolver; `githubrest` is unaffected and its tests need only a URL.
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
lifetime, since configuration is read once.

`CAPIClient` already pairs an `*http.Client` with a `host` field, so a client that carries its
own host is an established shape in the codebase.

#### Redirect policy

`WithCheckRedirect` sets the redirect policy for a client's requests. It was missed in the
original design, which has no way to express it: a redirect policy belongs to the
`http.Client`, not to a request, so no `RequestOption` can reach it. Two existing call sites
need it, and neither could migrate without it. `gh release download` rewrites a redirect
target to avoid legacy Codeload paths, and `gh repo delete` halts redirects with
`http.ErrUseLastResponse`.

The option shallow-copies the `http.Client` and sets the policy on the copy. That is not
ceremony: `NewClient` stores the caller's `*http.Client`, and the CLI hands the same one to
every command, so setting the field in place would install the policy process-wide for
unrelated commands. Both call sites already hand-roll the same copy for the same reason.

#### Which hosts may receive the token

A client will only send `Authorization` to a host it has been told to. The API base URL's own
host is credentialed automatically; `WithCredentialedHost` declares any others. An absolute
path aimed anywhere else is refused at `NewRequest`, before a request object exists, so the
token cannot be sent.

This exists because of the REST **upload** endpoint, which is the one place the CLI follows an
absolute URL to somewhere other than the API host. Whether it is a different host is a
deployment accident rather than anything meaningful:

| deployment | API base | uploads |
| --- | --- | --- |
| github.com | `https://api.github.com/` | `https://uploads.github.com/` |
| Enterprise Server | `https://HOST/api/v3/` | `https://HOST/api/uploads/` |

A rule of "same host as the base URL" would therefore pass on Enterprise Server and fail on
github.com for the identical call site, which is exactly the sort of difference a call site
should not have to know about. Declaring the upload host at construction, where the deployment
is already known, keeps it in one place.

A credentialed host reached over `http` when the client itself uses `https` is refused too,
since trusting the host says nothing about putting the token on the wire in clear.

This is narrower than it may sound. It constrains **absolute** paths only; relative paths
resolve against the base URL and are unaffected, which covers nearly every call site. Link
header pagination URLs point at the API host and so already pass.

It is worth being precise about what this does and does not add. Go's `http.Client` already
strips `Authorization` across a redirect to a different **hostname**, which was verified
empirically, so the redirect path was never the exposure. It does **not** strip across a port
change on the same hostname. The gap this closes is a directly supplied absolute URL, which
typically arrives from a response body rather than from the caller.

#### Relationship to `AddAuthTokenHeader`

Today the token is attached by `AddAuthTokenHeader`, a `RoundTripper` that derives the
hostname from the request (via `getHost`, which prefers `req.Host` and falls back to
`req.URL.Host`) and looks up that host's token, because `api.NewHTTPClient`
deliberately builds the underlying client with `Host: "none"` and `AuthToken: "none"` so go-gh
will not resolve them. That indirection exists to support a host-agnostic `*http.Client` that
is passed around generically.

`githubrest` sets `Authorization` itself, and the two coexist without removing the transport:

- The transport skips when the header is already set, so it never overwrites ours.
- On a cross-host redirect, Go's `http.Client` strips `Authorization` before the transport
  runs. This was verified empirically: a same-host redirect preserves the header, a
  cross-host redirect drops it. That is the same protection the transport's own
  `redirectHostnameChange` branch provides by hand, so the guarantee does not depend on which
  mechanism is in play.
- On such a redirect the transport then looks up a token for the new host, for example a CDN
  host serving a release asset, and finds none.

Removing `AddAuthTokenHeader` is therefore not required by this design and is out of scope.

What that transport does mean is that this package can express intent but not guarantee it. A
client built with `WithoutToken` sets no header, and `AddAuthTokenHeader` will then supply one
for the active user. That is why `WithoutToken` is documented as a statement of intent rather
than a promise of anonymity, and why closing the gap is listed as separate work rather than
claimed here.

### Requests

```go
type RequestOption func(*http.Request)

func WithHeader(name, value string) RequestOption
func WithSingleRequestToken(token string) RequestOption

func (c *Client) NewRequest(ctx context.Context, method, path string, body io.Reader, opts ...RequestOption) (*http.Request, error)

func (c *Client) NewUploadRequest(ctx context.Context, path string, open func() (io.ReadCloser, error), size int64, contentType string, opts ...RequestOption) (*http.Request, error)
```

`path` may be a path relative to the client's API base URL, or an absolute URL on a
credentialed host, as described under [Which hosts may receive the
token](#which-hosts-may-receive-the-token).

`NewRequest` sets `Authorization` from the client's token and then applies options, so an
option can override it. This makes token overrides explicit at the call site rather than
implicit in the transport's "do not overwrite" rule.

The request-level override is named `WithSingleRequestToken` rather than `WithToken`, since
`WithToken` is now the client-level `AuthStrategy` and the two would otherwise be one
identifier meaning different things at different scopes. It exists for a caller holding one
client that needs a particular request to carry different credentials; a caller whose every
request uses the same token says so at construction.

#### Uploads

`NewUploadRequest` builds a request that uploads a file, and takes every part of that as a
**required positional argument** rather than as options. It was not in the original design,
which assumed anything an upload needs is expressible as a `RequestOption`. That is true and
not sufficient: each part fails quietly, and somewhere else, when it is forgotten.

- No `ContentLength` sends chunked encoding, which the upload endpoint rejects.
- No `Content-Type` stores the asset as the wrong kind of file.
- No `GetBody` turns any redirect or retry into a failure, after the request itself looked
  fine.

An upload is a bundle that only works if all of it is applied, so it is the same argument as
`AuthStrategy`: make the compiler ask. google/go-github reaches the same conclusion with a
separate `NewUploadRequest`, though mostly for a different reason, since its `NewRequest`
takes `body any` and JSON-encodes it.

`open` is a `func() (io.ReadCloser, error)` rather than an `io.Reader`, so `GetBody` can
always be set. A reader can be consumed once, and a body that cannot be replayed is one
redirect away from failing. This is stricter than google/go-github, which sets `GetBody` only
when the reader happens to satisfy both `io.Seeker` and `io.ReaderAt`, making replayability
conditional on a type assertion the caller cannot see. It costs the CLI nothing:
`shared.AssetForUpload.Open` in `pkg/cmd/release/shared/upload.go` is already
`func() (io.ReadCloser, error)`, the exact signature.

An empty `contentType` is an error rather than a default, since `typeForFilename` at that call
site already falls back to `application/octet-stream` and nothing else is forced to guess.

Uploads route through the same resolution as `NewRequest`, so on github.com the client needs
`WithCredentialedHost("uploads.github.com")` and on Enterprise Server it needs nothing.

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

`path` is a `string`, matching `http.NewRequest`. `githubrest` does not depend on
`internal/safeurl`.

This is deliberate. The SafeURL guarantee is enforced at **domain helper signatures**, not at
the HTTP boundary: `downloadAsset(httpClient, assetURL safeurl.SafeURL, destPath string)`
constrains its untrusted input, then calls `assetURL.String()` one line before building the
request, because `http.NewRequest` takes a string. `githubrest.NewRequest` sits in exactly that
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

> **Migrating a call site to `githubrest` must not weaken an existing hardened parameter type.**
> An existing `safeurl.SafeURL` (or `safepaths.Absolute`) parameter stays as it is; the
> boundary lives on the domain helper, and `githubrest.NewRequest` receives the rendered string,
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

A discarded body is **drained** rather than merely closed. This was measured: closing a
response body without reading it never reuses the connection, and the transport does no
draining of its own even for a two byte body, so every subsequent request pays a fresh
handshake. Within `Do` this is an asymmetry rather than a general point, since the decoding
path already drains via `io.ReadAll`. Without the drain, passing `nil` would be quietly slower
than passing a typed value, which is a surprising thing to hang off an argument that reads as
"I do not care about the response".

An empty body with a **non-nil** `v` stays an error, but for a stated reason rather than by
inheritance from `api.Client.REST` and go-gh. A non-nil `v` says the caller expects a JSON
document, and JSON can already express "nothing" as `null`, which decodes cleanly and leaves
`v` at its zero value. A body with nothing in it at all is therefore a malformed response, not
an empty answer. google/go-github tolerates it by swallowing `io.EOF`, which suits a client
that covers the whole API generically and cannot audit each endpoint; here every call site
targets a known endpoint, and a zero-valued struct that looks like a real answer is the worst
outcome. The error names the status that arrived without a body, rather than surfacing
`json`'s "unexpected end of JSON input".

**`Do` returns the response alongside an error when one is available.** If the request
succeeded and only body handling failed, the status and headers are known good and the caller
should keep them. The body of such a response is already closed. Only a non-2xx status, or a
transport-level failure, yields a nil `*Response`.

### Errors

```go
type ErrorResponse struct {
    Errors     []ErrorItem
    Headers    http.Header
    Message    string
    RequestURL *url.URL
    StatusCode int
}

func NewErrorResponse(resp *http.Response) *ErrorResponse

func (e *ErrorResponse) Error() string
func (e *ErrorResponse) ScopesSuggestion() string
```

`NewErrorResponse` is exported so that migration step 4 can build one where a non-2xx response
is already in hand without routing it through `Send`. It takes a single argument, since
everything it needs is on the response.

Two details of go-gh's `HandleHTTPError`, which parses the payload, shape this. It reads the
body without closing it, which is why closing is `Send`'s responsibility, and it dereferences
`resp.Request.URL` unguarded. `net/http` does not populate `Response.Request` for a custom
`RoundTripper`, so that is a real nil dereference and not a theoretical one. `Send` backfills
`Request` on the response it owns; `NewErrorResponse` parses a shallow copy carrying a
placeholder, so a caller's response is never mutated behind its back.

`ScopesSuggestion` must not panic on a nil or zero-valued receiver, which migration step 4(a)
depends on.

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

`githubrest` keeps go-gh for everything substantial and replaces only its thin request wrapper:

| go-gh component | used by `githubrest` |
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

1. ~~Add `internal/githubrest` with no callers.~~ **Done.** The package exists and is
   unmigrated. Steps 2 to 7 below are unchanged.
2. Add `Factory.GitHubClient(host)`, which owns config lookup, canonical-host to API-host
   mapping, and token resolution.
3. Reimplement `api.Client.REST` and `RESTWithNext` on top of it. Two behaviours must be
   carried over deliberately rather than assumed:
   - go-gh's `DoWithContext` skips decoding on **204 and 205** (`StatusNoContent` and
     `StatusResetContent`). `RESTWithNext` handles only 204. Reimplementing both on one
     `githubrest.Do` must preserve each method's current behaviour, not unify them by accident.
   - Owning URL derivation means replicating go-gh's `restPrefix` special cases: `isGarage`,
     `auth.IsEnterprise`, and `github.localhost`, which yields **`http://`**, not `https://`.
     Because go-gh normalizes and `ghinstance.RESTPrefix` does not, picking either one changes
     the URL for `subdomain.github.com` relative to the other, so this step must state which
     rule it adopts and which call sites that moves.
4. Replace `api.HTTPError` with `githubrest.ErrorResponse`. There are 115 non-test references
   across 33 non-test files (181 across 53 including tests). Each declaration changes type,
   and because the new type is used as a pointer where the current one is a value, `errors.As`
   sites also gain a `*`. Field reads such as `.StatusCode` and `.Message` are unchanged, and
   the two `ScopesSuggestion()` call sites keep working because the method is retained. Every
   change is compiler-caught, with two exceptions that are not:

   **(a) This step must fix `internal/ghcmd/cmd.go`.** Line 243 calls
   `httpErr.ScopesSuggestion()` in an `else if` that is reached when `errors.As` returned
   **false**. That is harmless today only because `api.HTTPError` is a value type with a value
   receiver returning a stored string, so the zero value yields `""`. After this step
   `httpErr` is a `*githubrest.ErrorResponse`, nil on a failed `errors.As`, and the method is
   computed, so it dereferences `e.StatusCode` and `e.Headers` and panics. This fires on the
   common path of any non-API error reaching top-level handling. `authRecoveryCommand(cfg
   gh.Config, httpErr api.HTTPError)` takes a value too, so its signature changes as well.
   Both the pointer change and the computed-method change are individually safe; together
   they are a crash, so restructuring that `else if` chain is required work in this step and
   not a pre-existing issue to defer.

   **(b) `pkg/httpmock/stub.go` imports `api`** for
   `JSONErrorResponse(status int, err api.HTTPError)`. After this step it imports `githubrest`
   instead, so `githubrest`'s own in-package tests cannot use `httpmock`. Use an external
   `package githubrest_test` for any test that needs it.
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
the `*http.Client`, exactly as it is for `api.Client` today. Because `githubrest` takes a
resolved base URL and token, its own tests need neither configuration nor a factory.

Tests for `githubrest` cover: relative and absolute paths; absolute paths on undeclared hosts
being refused, and reachable once declared with `WithCredentialedHost`; `Authorization` set
from the client token, absent under `WithoutToken`, present but empty-valued under
`WithToken("")`, and overridden by `WithSingleRequestToken`; a `RequestOption` header
surviving rather than being replaced by go-gh's `headerRoundTripper`, which is the mechanism
that gives options precedence; `NewUploadRequest` setting length, content type and a `GetBody`
that genuinely replays, and rejecting each missing part; `Send` leaving the body open and `Do`
closing it, including `Send` closing it on the error path; `Do` with a nil `v` discarding the
body; 204 and 205 skipping decode; an empty body erroring while `null` decodes to the zero
value; 2xx status reaching the caller via `Response.StatusCode`; non-2xx producing an
`*ErrorResponse` with the decoded `message` and `errors`; `ScopesSuggestion` across the 4xx
range including 401 and excluding 422, and not panicking on a nil receiver; and `NextPage`
with a `Link` header present, absent, and containing multiple relations.

Every defensive branch was confirmed load-bearing by deliberately breaking it and checking
that a specific test failed, rather than by assuming coverage implies a meaningful assertion.

As built, all tests are in-package and use `httptest`, so the external-package constraint has
not bitten yet. It still applies: once step 4 lands, `pkg/httpmock` will import `githubrest`,
so any `githubrest` test that uses `httpmock` must live in an external
`package githubrest_test`.

Step 4 needs a regression test that a non-API error reaching `internal/ghcmd`'s error handling
does not panic, since that is the failure mode 4(a) introduces and the compiler cannot catch
it.

Factory tests cover canonical-host to API-host mapping for github.com, enterprise, tenancy,
garage and localhost, matching the existing `ghinstance.RESTPrefix` rules.

Existing `api` tests are the regression suite for step 3, and must pass unmodified.

## Divergences from the original design

Everything here was decided while building the package. The Design section above already
describes the code as it stands; this is the short list for anyone who read the original.

| Original | As built | Why |
| --- | --- | --- |
| package `githubv3` | package `githubrest` | The REST API is versioned by date now, so "v3" names the URL layout, not the API. |
| `NewClient(*url.URL, token string, *http.Client) *Client` | `NewClient(apiBaseURL string, *http.Client, AuthStrategy, ...ClientOption) (*Client, error)` | `*url.URL` does not carry absoluteness, so validation needs an error return regardless. |
| empty token means unauthenticated | `WithToken` and `WithoutToken`, both explicit and required | "I hold an empty token" and "I send no token" are different intentions, and one call site depends on the first. |
| absolute `path` unrestricted | absolute `path` must be on a credentialed host, over https | A directly supplied absolute URL was the one way to send the token somewhere unintended. |
| no redirect control | `WithCheckRedirect` | A redirect policy lives on the `http.Client`, so no `RequestOption` can reach it, and two call sites need it. |
| `WithToken` as a `RequestOption` | `WithSingleRequestToken` | `WithToken` became the client-level strategy; one name for two scopes would be worse than a longer one. |
| uploads via `RequestOption`s | `NewUploadRequest` | Expressible is not the same as safe: each forgotten part fails quietly and elsewhere. |
| `ErrorResponse` built only by `Send` | `NewErrorResponse` exported | Step 4 needs to build one from a response it already holds. |
| `Do` returns nil `*Response` with any error | returns the response when the request itself succeeded | Status and headers are known good when only body handling failed. |
| empty body error inherited from go-gh | same behaviour, stated reason, better message | `null` is how JSON says nothing; an empty body is malformed, and the caller should be told which status arrived without one. |

Two questions the original left open, answered after building it. Neither changed the code.

**Should `ErrorResponse` carry an `*http.Response`?** No. `Send` already hands the caller the
request and status before the error is built, and `Do` now returns the response alongside the
error whenever one exists, so the two paths that might have wanted it have it by other means.
The name still slightly over-promises, and the type still holds the response's content
destructured.

**Are `Send` and `Do` the right split?** Yes, but not for the reason originally given. The
original framed `Do` as convenience over `Send`. It is better understood as the only path that
guarantees the body is closed: `Send` hands that duty to the caller because
`apiLogFetcher.GetLog` genuinely needs it, and `Do` takes it back for everyone else.

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

Once `githubrest` exists it reduces to constructing a per-host client and calling `Send`, but
converting it is a follow-up rather than part of this work.

