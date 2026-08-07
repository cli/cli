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
`gh auth token --hostname github.com`. The login the test asserts against is
derived from that token, and can be set explicitly with
`GH_APIHOST_EXPECTED_LOGIN` to skip the lookup.

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

#### Expected results for phase 4 and 5

Every script is expected to be **green**. All the routing gaps the harness was
built to expose have been closed, so a red result now means a regression, not a
known gap.

| Test function  | Scripts                                                                | Expected  |
|----------------|------------------------------------------------------------------------|-----------|
| TestAPI        | basic-rest.txtar, basic-graphql.txtar                                  | green |
| TestReleases   | release-upload-download.txtar                                          | green |
| TestRepo       | repo-delete.txtar + three others                                       | green |
| TestWorkflows  | run-download.txtar                                                     | green |
| TestExtensions | extension.txtar                                                        | green |
| TestSearches   | search-issues.txtar                                                    | green |
| TestGists (phase 5) | gist-create-view-delete.txtar                                     | green |
| TestAuth       | auth-status.txtar                                                      | green |

Two failure signatures mean opposite things, and telling them apart is the first
step in any debugging session:

- `dial tcp 127.0.0.1:443: connection refused` for `api.github.com` means the code
  under test ignored `api_host` and went to the canonical host. This is a real
  product failure.
- `x509: certificate signed by unknown authority` for `gh-gateway.internal` means
  the request *reached* the gateway but the caller did not trust the harness CA.
  This is a harness defect, usually `SSL_CERT_FILE` not being propagated into a
  subprocess.

Note that `gh auth status` reports any transport failure as "The token in
hosts.yml is invalid". Do not read that message literally while debugging.

Two flakes are pre-existing and unrelated to `api_host`:
`repo-archive-unarchive` (server-side), and `search-issues`, which depends on the
search index catching up after the issue is created.

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
worktree and runs the acceptance subset there. It needs `HARNESS_COMMIT`: a
single squashed commit carrying the harness, which you build yourself.

**Build the harness commit fresh from current `HEAD` every time.** Do not reuse
one from a previous baseline run. A squashed commit is frozen at the moment it
was made and silently omits every harness fix landed since, which produces a
result that looks like a product failure but is not. The first baseline attempt
here used a commit predating a fix that propagated `SSL_CERT_FILE` into the
acceptance subprocess, and every script failed with `x509` errors that had
nothing to do with `api_host`.

The procedure, which is self-correcting because it starts from the current
harness:

```console
$ git checkout -b harness-vX.Y.Z HEAD
$ git revert --no-commit <the product fixes under test>
$ git commit -m "api_host gateway harness, squashed for cherry-picking"
$ git reset --soft <base> && git commit   # squash to one commit
```

Revert only the product fixes whose effect you are trying to measure, never the
harness itself. For a baseline that measures the routing gaps this branch
closed, that means reverting the `api.Client` migration commits and the `gh api`
relative-path fix, and leaving everything under `script/api-host-gateway/` and
`acceptance/` intact.

```console
$ HARNESS_COMMIT=<sha> \
    GH_APIHOST_ORG=my-org \
    script/api-host-gateway/at-rev.sh v2.97.0
```

Note that `at-rev.sh` runs the *old* revision's `run.sh`, not the current one.

`at-rev.sh` prompts for the org token the same way `run.sh` does. Set
`GH_APIHOST_ORG_TOKEN` in the environment to skip the prompt, which is worth
doing for a run across several revisions so it does not stop once per revision
to ask for a credential.

## Expected result today

All five phases pass, with every script in phases 4 and 5 green. See the table
above for how to interpret a failure.
