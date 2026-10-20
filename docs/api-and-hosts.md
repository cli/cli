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

## Global feature flags (provisional)

[`internal/globalflags`](../internal/globalflags/globalflags.go) is infrastructure
for global, unauthenticated flags. It does not enable or hide any production
commands, change telemetry rollout, or implement user-scoped flags.

The eventual production service is `cafe.github.com`, but **no global production
RPC is implemented or assumed to exist**. For trusted development only, set
`GH_GLOBAL_FEATURE_FLAG_SERVER_URL` to an HTTP(S) base URL serving this proposed
Twirp JSON contract:

```text
POST /twirp/github.cli.globalflags.v1.GlobalFeatureFlags/GetFlags
Content-Type: application/json

{}
```

```json
{"flags":{"example-command-released":true,"another-feature":false}}
```

The endpoint and messages are provisional, not an existing CAFE API. The response
is a complete snapshot: `flags` must be an object of nonblank names and boolean
values. An explicit `{"flags":{}}` clears all values. Unknown or removed flags
are disabled. Missing/null flags, null or non-boolean values, incomplete JSON,
non-200 responses, and responses exceeding 1 MiB are rejected as a whole.
URL credentials, query strings, fragments, and redirects are not supported.

Without the environment override, lookup and cache access are disabled. Routine
invocations do not contact a guessed production endpoint. With it, startup uses
the external HTTP client without tokens or an Authorization header. GitHub
accounts, `GH_HOST`, `GH_REPO`, `api_host`, and Proxima routing do not select or
authenticate this service.

The `ghcmd` global-flag startup module reads one `globalflags.Snapshot` from the
cache into `Factory.GlobalFlags` **before root command construction**. It never
waits for the network. `Main` supplies resolved startup inputs and retains only
debug diagnostic rendering. Future constructors can read
`f.GlobalFlags.Enabled("example-command-released")` without doing I/O. Snapshots
have no mutators and remain stable when the factory is copied. Tests can supply
values using `globalflags.NewSnapshot`; the zero value disables every flag.
Do not move loading into a pre-run hook: help bypasses it, and root construction
eagerly generates reference text. Help, reference, completion, and auth commands
all receive the startup snapshot. The internal `send-telemetry` and
`refresh-global-feature-flags` subprocesses skip startup lookup. Direct command
constructors, ordinary tests, and documentation generation do not bootstrap
flags or read the override.

The development cache is one `global-feature-flags/development-v1.json` file
under go-gh's cache directory (normally `$XDG_CACHE_HOME/gh` or `$HOME/.cache/gh`).
It contains the exact RPC endpoint, fetch time, and normalized flag snapshot.
Changing endpoints never reuses another endpoint's values, including on failure; a
successful fetch replaces the one development slot. Adjacent bounded state and
lock files throttle failed attempts and deduplicate concurrent refresh workers.
Inactive/default lookup never reads these files. Any future production cache
must use a separate namespace.

Cached snapshots are fresh for 30 minutes. A fresh cache is used without starting
another process. On a miss, corruption, or expiry, startup immediately uses
disabled defaults or the stale valid snapshot and starts the hidden detached
`refresh-global-feature-flags` worker. The current invocation never waits for
CAFE; successfully refreshed values apply to a later invocation, including when
the original command exits quickly. The worker has a two-second network deadline,
performs no GitHub authentication, records no telemetry, and is dispatched before
configuration loading, update checks, or command-tree construction. It does not
recursively bootstrap flags. A failed refresh is retried after a one-minute
cooldown rather than launching another process on every invocation, and it does
not add network latency to the caller. A worker that cannot be started releases
the cooldown so the next invocation can retry immediately.

Concurrent workers use a non-blocking filesystem lock to avoid duplicate
requests. Lock contention means another worker is already responsible for the
refresh. Filesystems without locking support continue in degraded mode and may
duplicate a request; other lock errors remain distinguishable. The lock is only
a fetch-deduplication optimization. The detached worker runs from the system
temporary directory so it does not retain the caller's working directory. A
closed temporary file is renamed over the cache so readers do not see partially
written JSON. The worker stores only the validated flag map rather than unrelated
response fields or whitespace. Cache reads are bounded to 2 MiB; invalid cache
data is ignored until a successful worker replaces it.

Startup reports cache-read or worker-start errors on stderr under `GH_DEBUG`,
without failing the requested command or changing its stdout. Detached workers
record their last failure in the bounded retry state so a subsequent invocation
can report it under `GH_DEBUG`; normal output remains quiet. The network deadline
does not bound filesystem latency. These flags are rollout hints, not
authorization or an immediate revocation mechanism.

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
