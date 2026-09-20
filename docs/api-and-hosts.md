# API and hosts

[Development guides](README.md)

Conventions for changing and reviewing HTTP requests, host selection,
authentication, and GitHub Enterprise Server (GHES) capability checks.

## Choose the host the operation actually targets

| Operation | Host source |
| --- | --- |
| Repository-scoped operation | Resolve the repository with the command's existing selection path, then use `repo.RepoHost()` |
| Explicit host flag or URL | Respect the command's existing validation and precedence; do not replace it with a default |
| Operation that needs a configured default | Use `cfg.Authentication().DefaultHost()` |

`ghinstance.Default()` always returns `github.com`; it is not user-selected
host resolution. Do not mechanically replace it with `DefaultHost()` either:
first determine whether the operation already has an explicit or repository
host. A default must not override a repository selected by `-R` or `GH_REPO`.

See `listRun`, `issueList`, and `milestoneByNumber` in
[issue/list](../pkg/cmd/issue/list/list.go) for resolved-repository host use,
and `AuthConfig.DefaultHost` in [config.go](../internal/config/config.go)
for configured default behavior. Preserve delayed Factory binding described in
[Command development](command-development.md#structure-and-lifecycle).

## Reuse clients, authentication, and request handling

Obtain the command's configured HTTP client from its existing dependency.
[`factory.New` and `HttpClientFunc`](../pkg/cmd/factory/default.go) wire
configuration, authentication, I/O, and transport behavior. Do not substitute a
bare client, copy token lookup, or hand-build an API endpoint for a normal
GitHub API operation.

[`api.NewClientFromHTTP`](../api/client.go) wraps that client. Reuse its
`GraphQL`/`Query`/`Mutate` and `REST` methods, or the existing request surface
appropriate to the response. Pass the chosen host and a relative API path.
Preserve endpoint-specific headers, scopes, redirect policy, error handling,
and pagination through the shared abstractions. Avoid extra round trips and
request only fields the command needs, including requested JSON export fields.

When changing API routing or constructing a client/request, also read
[`api_host`](api-host.md); consult its
[test harness](api-host-test-harness.md) when exercising routing changes.
The logical GitHub host still owns authentication, Git remotes, and web URLs;
an API gateway is not a replacement repository host. Do not assume every
command or service is already migrated or should use the gateway.

## Feature detection and cleanup

Reuse [`featuredetection.Detector`](../internal/featuredetection/feature_detection.go)
and existing capability definitions instead of scattered version tests.
Construct the detector for the target host using the existing client/cache
pattern. Propagate detection errors rather than treating them as unsupported.

Use temporary feature detection when an API is not yet generally available on
all supported GitHub API servers, including GHES. Skip new gates for
long-established APIs already available everywhere supported. Check current
capability documentation and support requirements; do not infer readiness from
GitHub.com behavior alone.

For a temporary gate that will eventually disappear, put
`// TODO <cleanupIdentifier>` directly above the `if` statement. Use the same
identifier across related call sites and capability definitions so the fallback
can be removed consistently. `IssueFeatures.ApiActorsSupported` in the detector
is an example of a capability with a documented cleanup condition. This is a
repository convention, not a claim about a particular linter.

Permanent gates for features that will not be supported on GHES do **not** need
a cleanup comment. Do not remove a gate just because one host supports it;
verify its cleanup condition across the current supported server range.

Before changing tests, read [Testing](testing.md). Cover the relevant selected
and default-host paths (including a non-`github.com` host), and supported versus
unsupported capability behavior. Assert the actual destination, not merely that
a mocked response was consumed.
