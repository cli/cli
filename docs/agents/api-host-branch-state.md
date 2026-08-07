# State of the `api_host` / flexible `api.Client` branch

Working notes for branch `williammartin-flexible-api-client-surface`. Written to be
durable: an agent or human picking this up cold should be able to reconstruct what
was done, what was proven, and what is still broken, without replaying the session.

- Branch tip at time of writing: `143c3d54618523a84144073da424005e204bec3e`
- Branched from trunk at `5ccd971df4421952a7448e62547b77bf6c9e1ef7`
- Trunk reference used for "before" permalinks: `83c6321b8faba2ec6202af70b1cc0e2ed936495e`
- Tracking issues: [#13991](https://github.com/cli/cli/issues/13991) (route raw
  `httpClient.Do` through `api.Client`), serving
  [#13717](https://github.com/cli/cli/issues/13717) (per-host `api_host`)
- Companion go-gh change: [cli/go-gh#275](https://github.com/cli/go-gh/pull/275) (draft)

## Why the two halves are one branch

`api_host` can only be honoured centrally if every request goes through a single
chokepoint. Trunk had 36 raw `httpClient.Do(req)` call sites building absolute
`https://api.github.com/...` URLs, each of which bypasses any central resolution.
So the branch does two things that only make sense together:

1. Give `api.Client` a request surface flexible enough that those call sites can
   actually move onto it (per-request headers, redirect policy, context, raw
   `*http.Response`).
2. Migrate the call sites, then prove with a test harness that nothing still
   escapes to `api.github.com`.

Without (2) the migration is unfalsifiable: a site that ignores `api_host` still
passes every existing test, because `api.github.com` is reachable in CI.

## The test harness

Lives in `script/api-host-gateway/`. Entry point `run.sh`; assertions in `test.sh`;
the proxy in `gateway/`; `at-rev.sh` runs the harness against an arbitrary revision.

The core idea is a **blackhole plus a recording proxy**:

- A TLS reverse proxy (`gh-gateway.internal`) sits in front of the real
  `api.github.com`, logging every request it sees: method, path, `Host`, whether an
  `Authorization` header was present, and the response status.
- `api.github.com` is blackholed inside the container by pointing it at
  `127.0.0.1:443`, where nothing listens.
- `api_host` is configured to point at the gateway.

The consequence is the whole point: a request that respects `api_host` reaches the
gateway and is logged; a request that ignores it dies with
`dial tcp 127.0.0.1:443: connection refused`. There is no silent pass.

The two failure signatures are worth memorising, because they mean opposite things:

| Signature | Meaning |
|---|---|
| `connection refused` on `127.0.0.1:443` for `api.github.com` | genuine bypass; the code under test ignored `api_host` |
| `x509: certificate signed by unknown authority` for `gh-gateway.internal` | the request *reached* the gateway; this is a harness defect (missing `SSL_CERT_FILE`), not a product signal |

Five phases:

1. **`api_host` set, `api.github.com` blackholed.** Direct assertions on `gh api`
   (REST and GraphQL), `gh repo view`, and `--paginate` (which must follow a
   *rewritten* `Link` header), plus assertions against the gateway log that the
   requests really arrived and carried a token.
2. **No `api_host`, no blackhole.** Control: everything still works, and the
   gateway sees *nothing*. Guards against the harness accidentally forcing traffic
   through the proxy.
3. **No `api_host`, blackholed.** Sanity: `gh` must fail. Guards against the
   blackhole silently not working, which would make phase 1 meaningless.
4. **Org-scoped acceptance subset** through the gateway (7 test functions,
   11 scripts) under a fine-grained PAT owned by the test org.
5. **User-scoped acceptance subset** (the gist script) under an OAuth token.

Phases 4 and 5 are split because a fine-grained PAT has exactly one resource owner,
and Gists is an *Account* permission while the rest need *Organization* permissions.
No single PAT covers both.

The subset was chosen to cover the migrated call sites:

| Test function | Scripts |
|---|---|
| `TestAPI` | `basic-rest`, `basic-graphql` |
| `TestReleases` | `release-upload-download` |
| `TestRepo` | `repo-delete`, `repo-list-rename`, `repo-read-file`, `repo-rename-transfer-ownership` |
| `TestWorkflows` | `run-download` |
| `TestExtensions` | `extension` |
| `TestSearches` | `search-issues` |
| `TestAuth` | `auth-status` |
| `TestGists` (phase 5) | `gist` |

`run_subset` distinguishes three outcomes, not two: red, green, and **matched no
tests**. The third matters because a mistyped script name would otherwise look
green. It is detected by grepping for `[no tests to run]`.

### Running it

```sh
GH_APIHOST_ORG=gh-acceptance-testing GH_APIHOST_ACCEPTANCE=yes script/api-host-gateway/run.sh
```

It prompts for a fine-grained PAT (it cannot list the randomly named repos the
scripts create, so the PAT must be scoped to all repositories in the org).

To run the harness against an older revision, use `at-rev.sh REV`. It cherry-picks
a squashed harness commit onto that revision. **Any squashed harness commit is
frozen in time and silently omits later harness fixes.** The first baseline
attempt used a commit that predated the `SSL_CERT_FILE` propagation fix and
produced an entirely bogus all-x509 result. Rebuild the harness commit from
current `HEAD`, reverting only the fixes under test.

## Baseline: v2.97.0

Baseline harness built from then-current `HEAD` with only the product fixes under
test reverted, then squashed and cherry-picked onto the tag. The commit itself is
not published; see `script/api-host-gateway/README.md` for how to rebuild one,
and why reusing an old harness commit produces a bogus result.

**Phases 1-3:** 8 assertions failed. **Phases 4-5:** 7 of 7 org test functions red;
`TestGists` correctly reported as *absent* (the script does not exist at that tag).

Zero `x509` occurrences. Every single failure is `connection refused` against
`api.github.com`, so every red is a genuine `api_host` bypass.

Note that `gh repo create` **succeeds** at baseline. That is expected and is what
makes the result precise: go-gh's own client plus the token-forwarding fix already
handle the plain path, so the scripts run deep into their bodies and fail
specifically at the raw `.Do` sites. The reds isolate exactly the migration target.

The nine distinct bypasses observed:

`gh api` (REST), `gh api graphql`, `gh release create`, `gh repo delete`,
`gh api -X PUT` (contents), `gh repo rename` (PATCH), `gh run download`
(artifacts), `gh repo edit --add-topic`, `gh search issues`, `gh auth status`.

`gh auth status` reports transport failures as "The token in hosts.yml is invalid",
which masks the real cause. Do not read that message literally.

## Current state: this branch

With go-gh pinned to `v2.13.1-0.20260807082810-f5ea1c2fae5f` (the head of
[go-gh#275](https://github.com/cli/go-gh/pull/275)):

| | v2.97.0 | this branch |
|---|---|---|
| Phases 1-3 | 8 assertions failed | all pass |
| Phases 4-5 | 7 of 7 red | 12 of 12 scripts green |
| `TestGists` | absent | green |

The go-gh side of this needs two things, both in #275: per-host API endpoint
overrides, and a configurable redirect policy. The redirect policy is needed
because go-gh's default client follows redirects itself, which swallows the 301 that
`gh repo delete` needs to observe in order to report a transferred repository.

Strictly speaking, redirect control is not required for `api_host` in the narrow
sense: `gh api` already honours `api_host` today with its own hand-rolled client
(see `pkg/cmd/api/api.go`, which calls `APIHostForHost` directly). It *is* required
for `api_host` to be honoured **centrally**, which is the actual goal here.

## What changed, by command

Most entries moved from a raw `httpClient.Do(req)` on an absolute
`https://api.github.com/...` URL, to an `api.Client` call taking a relative path
plus a hostname, so central resolution applies. A few were already on `api.Client`
but passed an absolute URL built from `ghinstance.RESTPrefix`, which bypasses
resolution just as effectively; those moved to relative paths.

One deliberate exception: `deleteRelease` still passes an absolute URL, because the
release URL is supplied by the API response itself. That is safe precisely because
the response came from the configured `api_host`, so the URL already points there.

The new surface on `api.Client`
([api/client.go](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/api/client.go)):

| Symbol | Line |
|---|---|
| `RequestOption` | [L56](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/api/client.go#L56) |
| `WithHeader` | [L67](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/api/client.go#L67) |
| `WithoutFollowingRedirects` | [L82](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/api/client.go#L82) |
| `WithEndpointScopes` | [L91](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/api/client.go#L91) |
| `Request` | [L207](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/api/client.go#L207) |
| `RequestWithContext` | [L222](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/api/client.go#L222) |
| `DoRequest` (escape hatch for callers that must build their own `*http.Request`) | [L265](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/api/client.go#L265) |

`WithHeader` was the design crux. A verbatim copy of go-gh's
`RESTClient.Request(method, path, body)` would not have worked: go-gh's headers are
client-level only, and per-request headers were the single largest blocker in the
census (10 of 26 in-scope sites).

Central resolution happens in
[api/http_client.go](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/api/http_client.go#L172)
via `HostForAPIHost`, with the config side in
[internal/config/config.go](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/internal/config/config.go#L332).

### Migrated call sites

| Command(s) | File | Before (trunk) | After (branch) |
|---|---|---|---|
| `gh release view/download` | `pkg/cmd/release/shared/fetch.go` | [L150](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/release/shared/fetch.go#L150) | [L147](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/release/shared/fetch.go#L147) |
| `gh release create/edit` | `pkg/cmd/release/create/http.go` | [L124](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/release/create/http.go#L124) | [L122](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/release/create/http.go#L122) |
| `gh release delete` | `pkg/cmd/release/delete/delete.go` | [L128](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/release/delete/delete.go#L128) | [L129](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/release/delete/delete.go#L129), [L147](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/release/delete/delete.go#L147) |
| `gh release download` | `pkg/cmd/release/download/download.go` | [L326](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/release/download/download.go#L326) | [L336](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/release/download/download.go#L336) |
| `gh release upload` | `pkg/cmd/release/shared/upload.go` | [L189](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/release/shared/upload.go#L189) | [L191](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/release/shared/upload.go#L191) |
| `gh repo read-file` | `pkg/cmd/repo/read-file/http.go` | [L100](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/repo/read-file/http.go#L100) | [L96](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/repo/read-file/http.go#L96) |
| `gh repo delete` | `pkg/cmd/repo/delete/http.go` | [L29](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/repo/delete/http.go#L29) | [L17](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/repo/delete/http.go#L17) |
| `gh repo edit` | `pkg/cmd/repo/edit/edit.go` | [L581](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/repo/edit/edit.go#L581), [L623](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/repo/edit/edit.go#L623) | [L578](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/repo/edit/edit.go#L578), [L618](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/repo/edit/edit.go#L618) |
| `gh repo garden` | `pkg/cmd/repo/garden/http.go` | [L90](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/repo/garden/http.go#L90) | [L88](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/repo/garden/http.go#L88) |
| `gh repo rename`, `RepoExists` (and other `api` package helpers) | `api/queries_repo.go` | [L650](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/api/queries_repo.go#L650), [L1674](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/api/queries_repo.go#L1674) | [L655](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/api/queries_repo.go#L655), [L1680](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/api/queries_repo.go#L1680) |
| `gh extension install/upgrade` | `pkg/cmd/extension/http.go` | [L29](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/extension/http.go#L29) | [L26](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/extension/http.go#L26) |
| `gh run download` | `pkg/cmd/run/download/http.go` | [L41](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/run/download/http.go#L41) | [L40](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/run/download/http.go#L40) |
| `gh run view --log` | `pkg/cmd/run/view/logs.go` | [L55](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/run/view/logs.go#L55) | [L51](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/run/view/logs.go#L51) |
| `gh gist create/edit/view/list` | `pkg/cmd/gist/shared/shared.go` | [L264](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/gist/shared/shared.go#L264) | [L264](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/gist/shared/shared.go#L264) |
| `gh pr diff` | `pkg/cmd/pr/diff/diff.go` | [L230](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/pr/diff/diff.go#L230) | [L225](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/pr/diff/diff.go#L225) |
| `gh auth login/status/refresh` | `pkg/cmd/auth/shared/oauth_scopes.go` | [L49](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/cmd/auth/shared/oauth_scopes.go#L49) | [L38](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/cmd/auth/shared/oauth_scopes.go#L38) |
| `gh search *` | `pkg/search/searcher.go` | [L249](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/pkg/search/searcher.go#L249) | [L248](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/pkg/search/searcher.go#L248) |
| update checker (all commands) | `internal/update/update.go` | [L128](https://github.com/cli/cli/blob/83c6321b8faba2ec6202af70b1cc0e2ed936495e/internal/update/update.go#L128) | [L130](https://github.com/cli/cli/blob/143c3d54618523a84144073da424005e204bec3e/internal/update/update.go#L130) |

Two entries look like they contradict the acceptance results and are worth calling
out, because the mismatch has already cost one investigation: `gh repo rename` and
`gh search` are red at baseline and green at tip, yet **no file under
`pkg/cmd/repo/rename/` or `pkg/cmd/search/` changed**. They were migrated
indirectly, via `api/queries_repo.go` and `pkg/search/searcher.go` respectively.

## Known gaps: things that still ignore `api_host`

These are *not* covered by this branch and will still bypass a configured
`api_host`. Each was checked against the code, not assumed.

1. **Codespaces** (`internal/codespaces/api/api.go`). Builds its own absolute base
   URL from `GITHUB_API_URL` or `ghinstance.RESTPrefix(host)` and calls
   `httpClient.Do` directly. Has its own client construction, its own tracing
   wrapper, and its own auth. Migrating it is a self-contained piece of work.
2. **Agent tasks / CAPI** (`pkg/cmd/agent-task/capi/`). Talks to a *different*
   service with its own base URL, injected via `newCAPITransport`. Arguably out of
   scope for `api_host` entirely, but should be an explicit decision rather than an
   omission.
3. **Barista telemetry** (`internal/barista/observability/telemetry.twirp.go`).
   Generated Twirp code, not the GitHub API. Out of scope.
4. **`gh api` itself** (`pkg/cmd/api/http.go`). Still calls `client.Do` with a
   hand-built request, because `--input` needs to control `Content-Length`. It
   *does* honour `api_host`, but by calling `APIHostForHost` itself rather than
   through central resolution. It is correct today and fragile tomorrow: it is a
   second implementation that must be kept in sync.
5. **`gh auth status` error reporting.** Any transport failure surfaces as "The
   token in hosts.yml is invalid", which actively misleads during `api_host`
   debugging. Cosmetic but costly.

## Other things worth knowing

- **A `replace` directive substitutes, it does not merge.** A local `replace`
  pointing at a go-gh clone branched from go-gh trunk silently removed `api_host`
  support entirely. The symptom was phase 1 GraphQL failing, which no REST-only
  change could possibly cause. Pin via the module proxy instead.
- **`testscript` builds its environment from a fixed allowlist**
  (`testscript.go:466-496`), propagating only `GOCOVERDIR` and `GORACE`.
  `SSL_CERT_FILE` had to be propagated explicitly in `sharedSetup`.
- **Pre-existing flakes**, unrelated to this work: `repo-archive-unarchive`
  (server-side), and `search-issues`, which depends on the search index catching
  up. The latter's sleep was raised to 20s on this branch.
