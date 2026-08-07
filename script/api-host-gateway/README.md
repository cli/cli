# `api_host` gateway test

A black box test for routing `gh` API traffic through a corporate gateway, as
proposed in [cli/cli#13717](https://github.com/cli/cli/issues/13717) and
implemented for go-gh in [cli/go-gh#275](https://github.com/cli/go-gh/pull/275).

It runs the real `gh` binary against a recording TLS reverse proxy that forwards
to the real api.github.com, with `api.github.com` blackholed so `gh` has no way
to reach GitHub except through the gateway. It then asserts both halves of the
claim: the gateway saw the requests, and `gh` got real answers back.

## Running it

```console
$ script/api-host-gateway/run.sh
```

Requires Docker and a token: `$GH_TOKEN` if set, otherwise
`gh auth token --hostname github.com`. The token's account must match the login
the test expects, which defaults to `williammartin` and can be overridden with
`GH_APIHOST_EXPECTED_LOGIN`.

### Running with the acceptance subset (phase 4)

Set `GH_APIHOST_ACCEPTANCE=yes` and `GH_APIHOST_ORG` to a GitHub organisation
the token can create repositories in:

```console
$ GH_APIHOST_ACCEPTANCE=yes GH_APIHOST_ORG=my-org script/api-host-gateway/run.sh
```

Phase 4 creates real repositories and can take up to 45 minutes. It is
intentionally non-blocking: a red result is printed but does not stop the script
or affect the phases 1-3 exit code.

## Why a container

Two constraints make this awkward to run directly on a developer machine, and
trivial inside a Linux container:

- `api_host` is a bare hostname, so it cannot carry a port. The gateway has to
  listen on 443, which needs root.
- The gateway's certificate has to be trusted by `gh`. Go honours
  `SSL_CERT_FILE` on Linux but not on macOS, where it uses the platform
  verifier, so on macOS the only alternative would be installing a CA into the
  keychain.

The container also gives us a writable `/etc/hosts`, which is how
`api.github.com` gets blackholed.

## What it does

`run.sh` starts `golang:1.26` with the repository mounted at `/src` and runs
`test.sh` inside it. `test.sh`:

1. Builds `gh` and the gateway.
2. Resolves `api.github.com` to an IP and starts the gateway on
   `127.0.0.2:443`, pinned to that IP so it keeps working after the blackhole
   goes in. The gateway generates its own CA and leaf certificate for
   `gh-gateway.internal`.
3. Trusts that CA through `SSL_CERT_FILE`, and points `gh-gateway.internal` at
   `127.0.0.2` in `/etc/hosts`.
4. Writes an isolated `GH_CONFIG_DIR` whose `hosts.yml` has `github.com` with a
   `user`, an `oauth_token`, and `api_host: gh-gateway.internal`.
5. Runs three phases of assertions.

### Phases

**Phase 1, routed.** `api_host` is set and `api.github.com` is blackholed. Each
of `gh api user`, `gh api repos/cli/cli`, `gh api graphql`, `gh repo view` and
`gh api --paginate` must return real GitHub data, and the gateway must have
recorded the matching request with `Host: gh-gateway.internal` and an
`Authorization` header. The paginated case additionally proves the gateway's
`Link` header rewriting works, because the second page can only be fetched if
`gh` was sent back to the gateway rather than to `api.github.com`, and that the
follow-up request still carries the token. That last assertion is easy to fail:
`gh` attaches tokens by request host, and the gateway host has no token of its
own, so a naive implementation paginates anonymously and only appears to work
against public resources.

**Phase 2, control.** No `api_host` and no blackhole. The same commands must
still work and the gateway must record nothing, so the override is what causes
the routing.

**Phase 3, blackhole sanity.** No `api_host`, blackhole back on. `gh api user`
must fail. Without this, phase 1 could pass through a direct connection that the
blackhole was silently failing to prevent.

**Phase 4, acceptance subset (optional).** Runs when `GH_APIHOST_ACCEPTANCE=yes`.
`api_host` is set and `api.github.com` is blackholed, matching phase 1. Eight
`go test` invocations run a chosen subset of the real acceptance suite through
the gateway. A red result is printed but does not abort the script, because the
phase exists to produce a tally that goes from red to green over time, not to
gate the build.

#### Predicted results for phase 4 (as of 2026-08-06)

| Test function  | Scripts                                                                | Expected  |
|----------------|------------------------------------------------------------------------|-----------|
| TestAPI        | basic-rest.txtar, basic-graphql.txtar                                  | **GREEN** |
| TestReleases   | release-upload-download.txtar                                          | red       |
| TestRepo       | repo-delete.txtar + three others                                       | red       |
| TestWorkflows  | run-download.txtar                                                     | red       |
| TestExtensions | extension.txtar                                                        | red       |
| TestGists      | gist-create-view-delete.txtar                                          | red       |
| TestSearches   | search-issues.txtar                                                    | red       |
| TestAuth       | auth-status.txtar                                                      | red       |

**Why TestAPI is green.** Task 4b made `gh api` honour `api_host` for relative
paths, so `basic-rest.txtar` and `basic-graphql.txtar` now route through the
gateway correctly.

**Why the rest are red.** The common root cause is `gh repo delete`, which still
builds an absolute URL via `DoRequest` for its redirect policy. `DoRequest`
deliberately does not rewrite to `api_host`. Most acceptance scripts end in a
`defer gh repo delete` cleanup, so they fail in cleanup even when the command
under test routes correctly. Fixing this requires a `CheckRedirect` field on
go-gh's `ClientOptions` (the sibling change to cli/go-gh#275), not a cli/cli
change.

A red result in phase 4 is therefore expected and is **not** a signal of a
broken harness unless it appears in TestAPI, which was fixed. A new green result
in any other test is a sign that the underlying routing gap has been closed.

## The gateway

`gateway/main.go` is a single dependency-free program. Beyond recording
requests, it buffers each response and rewrites every occurrence of
`api.github.com` to `gh-gateway.internal`, in headers such as `Link` and in JSON
bodies. That mirrors how this is handled in practice: GitHub returns absolute
URLs on the canonical host, so a gateway that does not rewrite them sends
clients straight back off its route. It asks the upstream for an identity
content encoding so the body is rewritable, and fixes `Content-Length`
afterwards.

## Debugging the gateway on its own

The gateway does not need root or a container if you give it an unprivileged
port, which makes it easy to poke at with `curl`:

```console
$ go build -o /tmp/gateway ./script/api-host-gateway/gateway
$ /tmp/gateway -listen 127.0.0.1:8443 \
    -upstream-addr "$(dig +short api.github.com | head -1):443" \
    -ca-out /tmp/ca.pem -log /tmp/gateway.jsonl &
$ curl --cacert /tmp/ca.pem --resolve gh-gateway.internal:8443:127.0.0.1 \
    -H "Authorization: token $(gh auth token)" \
    https://gh-gateway.internal:8443/user
```

`gh` itself cannot be pointed at that, because `api_host` cannot carry a port.

## Running against an older revision

`at-rev.sh` applies the harness onto an arbitrary revision in a detached
worktree and runs the acceptance subset there:

```console
$ HARNESS_COMMIT=0e661576450b4918dabbe7b54952ea3f19c37187 \
    GH_APIHOST_ORG=my-org \
    script/api-host-gateway/at-rev.sh v2.97.0
```

`HARNESS_COMMIT` is the squashed harness commit (`0e661576450b4918dabbe7b54952ea3f19c37187`)
on branch `api-host-gateway-harness`. It contains everything on this branch
except Task 4b (`ef1f342f5` and `c51147d6c`). Task 4b is deliberately absent:
it makes `gh api` honour `api_host` for relative paths, so carrying it onto
`v2.97.0` would make TestAPI green in the baseline and hide one of the reds the
baseline exists to measure. Do not "fix" this omission.

`at-rev.sh` prompts for the org token the same way `run.sh` does. Set
`GH_APIHOST_ORG_TOKEN` in the environment to skip the prompt, which is worth
doing for a run across several revisions so it does not stop once per revision
to ask for a credential.

## Expected result today

Phases 1, 2 and 3 pass. Phase 4, when enabled, is a tally run: TestAPI is
expected to pass; the rest are expected to fail for reasons documented in the
phase 4 table above.
