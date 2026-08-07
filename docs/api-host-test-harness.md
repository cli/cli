# The `api_host` test harness

A black box test for routing `gh` API traffic through a corporate gateway, as
proposed in [cli/cli#13717](https://github.com/cli/cli/issues/13717).

The harness lives in `script/api-host-gateway/`.

## Why it exists

`api_host` can only be honoured centrally if every request goes through a single
chokepoint. A call site that builds an absolute `https://api.github.com/...` URL
and calls `httpClient.Do` bypasses any central resolution, and no existing test
notices, because `api.github.com` is reachable from CI. The migration is
unfalsifiable without something that makes a bypass fail loudly.

This harness is that something. It runs the real `gh` binary against a recording
TLS reverse proxy that forwards to the real `api.github.com`, with
`api.github.com` blackholed so `gh` has no way to reach GitHub except through the
gateway. It then asserts both halves of the claim: the gateway saw the request,
and `gh` got a real answer back.

The consequence is the whole point. A request that respects `api_host` reaches
the gateway and is logged. A request that ignores it dies with
`dial tcp 127.0.0.1:443: connection refused`. There is no silent pass.

## Running it

```console
$ script/api-host-gateway/run.sh
```

Requires Docker and a token: `$GH_TOKEN` if set, otherwise
`gh auth token --hostname github.com`. The login the test asserts against is
derived from that token, and can be set explicitly with
`GH_APIHOST_EXPECTED_LOGIN` to skip the lookup.

### It tests your working tree, not your HEAD

The container builds `gh` from the working tree that contains `run.sh`. If that
tree is dirty, the result describes neither `HEAD` nor anything else nameable,
which is worthless for a test whose entire job is to tell you which call sites
are wrong. So `run.sh` refuses to start on a dirty tree, and prints the revision
it is about to test:

```console
$ script/api-host-gateway/run.sh
Testing gh at 5fd4e7f27
```

To test a revision without disturbing your current work, run it from a second
checkout:

```console
$ git worktree add /tmp/gh-harness <revision>
$ /tmp/gh-harness/script/api-host-gateway/run.sh
```

`GH_APIHOST_ALLOW_DIRTY=yes` runs anyway and marks the revision `-dirty`. That is
useful while iterating on a fix, but a `-dirty` run should never be quoted as
evidence that a call site is fixed.

### With the acceptance subset

Set `GH_APIHOST_ACCEPTANCE=yes` and `GH_APIHOST_ORG` to a GitHub organisation the
token can create repositories in:

```console
$ GH_APIHOST_ACCEPTANCE=yes GH_APIHOST_ORG=my-org script/api-host-gateway/run.sh
```

This creates real repositories and takes a couple of minutes. It prompts for a
fine-grained PAT owned by the organisation. The PAT must be scoped to all
repositories in the org, because the scripts create repositories with random
names that cannot be listed ahead of time. Set `GH_APIHOST_ORG_TOKEN` to skip the
prompt.

Phases 4 and 5 are intentionally non-blocking: a red result is printed but does
not stop the script or affect the exit code, which is driven by phases 1-3 alone.
The phases exist to produce a tally that moves from red to green over a series of
changes, not to gate a build.

## Why a container

Two constraints make this awkward to run directly on a developer machine, and
trivial inside a Linux container:

- `api_host` is a bare hostname, so it cannot carry a port. The gateway has to
  listen on 443, which needs root.
- The gateway's certificate has to be trusted by `gh`. Go honours `SSL_CERT_FILE`
  on Linux but not on macOS, where it uses the platform verifier, so on macOS the
  only alternative would be installing a CA into the keychain.

The container also gives us a writable `/etc/hosts`, which is how
`api.github.com` gets blackholed.

## What it does

`run.sh` starts `golang:1.26` with the repository mounted at `/src` and runs
`test.sh` inside it. `test.sh`:

1. Builds `gh` and the gateway.
2. Resolves `api.github.com` to an IP and starts the gateway on `127.0.0.2:443`,
   pinned to that IP so it keeps working after the blackhole goes in. The gateway
   generates its own CA and leaf certificate for `gh-gateway.internal`.
3. Trusts that CA through `SSL_CERT_FILE`, and points `gh-gateway.internal` at
   `127.0.0.2` in `/etc/hosts`.
4. Writes an isolated `GH_CONFIG_DIR` whose `hosts.yml` has `github.com` with a
   `user`, an `oauth_token`, and `api_host: gh-gateway.internal`.
5. Runs the phases below, recording every assertion by name.

### Phases

**Phase 1, routed.** `api_host` is set and `api.github.com` is blackholed. Each of
`gh api user`, `gh api repos/cli/cli`, `gh api graphql`, `gh repo view` and
`gh api --paginate` must return real GitHub data, and the gateway must have
recorded the matching request with `Host: gh-gateway.internal` and an
`Authorization` header.

The paginated case additionally proves the gateway's `Link` header rewriting
works, because the second page can only be fetched if `gh` was sent back to the
gateway rather than to `api.github.com`, and that the follow-up request still
carries the token. That last assertion is easy to fail: `gh` attaches tokens by
request host, and the gateway host has no token of its own, so a naive
implementation paginates anonymously and only appears to work against public
resources.

**Phase 2, control.** No `api_host` and no blackhole. The same commands must still
work and the gateway must record nothing, so the override is demonstrably what
causes the routing.

**Phase 3, blackhole sanity.** No `api_host`, blackhole back on. `gh api user` must
fail. Without this, phase 1 could be passing through a direct connection that the
blackhole was silently failing to prevent.

Phases 2 and 3 are controls. They are expected to pass even when phase 1 is
entirely red, and if they ever fail the harness itself is broken rather than the
product.

**Phases 4 and 5, acceptance subset.** Run when `GH_APIHOST_ACCEPTANCE=yes`.
`api_host` is set and `api.github.com` is blackholed, matching phase 1. A chosen
subset of the real acceptance suite runs through the gateway, one script per
`go test` invocation.

Scripts are run one at a time rather than batched per test function, because a
test function bundles scripts that fail for unrelated reasons. Batching them
would hide a single script turning green.

The two phases are split because a fine-grained PAT has exactly one resource
owner, and Gists is an *Account* permission while the rest need *Organization*
permissions. No single PAT covers both. Phase 4 runs the org-scoped scripts under
a PAT owned by the test organisation; phase 5 runs the gist script under the
developer's own OAuth token.

`run_subset` distinguishes three outcomes, not two: pass, fail, and **matched no
tests**. The third matters because a script that does not exist at the revision
under test would otherwise look green. It is detected by grepping for
`[no tests to run]`.

### Reading a failure

Two failure signatures mean opposite things, and telling them apart is the first
step in any debugging session:

| Signature | Meaning |
|---|---|
| `dial tcp 127.0.0.1:443: connection refused` for `api.github.com` | The code under test ignored `api_host` and went to the canonical host. A real product failure. |
| `x509: certificate signed by unknown authority` for `gh-gateway.internal` | The request *reached* the gateway but the caller did not trust the harness CA. A harness defect, usually `SSL_CERT_FILE` not being propagated into a subprocess. |

Note that `gh auth status` reports any transport failure as "The token in
hosts.yml is invalid". Do not read that message literally while debugging.

Two flakes are pre-existing and unrelated to `api_host`: `repo-archive-unarchive`
(server-side), and `search-issues`, which depends on the search index catching up
after the issue is created.

## The gateway

`gateway/main.go` is a single dependency-free program. Beyond recording requests,
it buffers each response and rewrites every occurrence of `api.github.com` to
`gh-gateway.internal`, in headers such as `Link` and in JSON bodies. That mirrors
how this is handled in practice: GitHub returns absolute URLs on the canonical
host, so a gateway that does not rewrite them sends clients straight back off its
route. It asks the upstream for an identity content encoding so the body is
rewritable, and fixes `Content-Length` afterwards.

Content hosts are deliberately left reachable. `api_host` proxies the API, not
every GitHub endpoint: gist file bodies come from `gist.githubusercontent.com`,
and the same applies to `codeload` and release binary storage. The harness
blackholes only `api.github.com`, which models a customer's network correctly.
Blackholing the content hosts as well would produce failures that no user would
ever hit.

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

## Current state

Phases 1, 2 and 3 pass, unchanged.

`gist-create-view-delete` is green. It is the only script whose cleanup is
`gh gist delete` rather than `gh repo delete`, which is why it can go green on
its own while the rest cannot.

Nine scripts are still red, and the count overstates how much is broken. Five
fail in their body, on a call site that is genuinely still unrouted. The other
four pass their body in full and fail only in deferred cleanup:

| Script | Fails at |
|---|---|
| `repo-delete` | `gh repo delete`, line 9 |
| `auth-status` | scope checking, line 2 |
| `repo-read-file` | `gh repo read-file`, line 34 |
| `extension` | `gh repo edit --add-topic`, line 29 |
| `search-issues` | `gh search issues`, line 21 |
| `release-upload-download`, `repo-list-rename`, `repo-rename-transfer-ownership`, `run-download` | nothing in the body; cleanup only |

Almost every script creates a repository and defers `gh repo delete`, so until
that one call site is routed those four cannot report anything but red, however
much of their own subject matter works. Script colour is a poor measure of
progress on its own: read the step list above the failure, which shows how far
the body got, and the `.txtar:NN` line, which says whether the failure is the
script's subject or its cleanup.

## Remaining

`gh repo delete`, which needs control over redirects, and `gh auth status`,
`gh repo read-file`, `gh repo edit` and `gh search issues`, which need
per-request headers.

## Transcript

From a `GH_APIHOST_ACCEPTANCE=yes` run of this commit. Each commit that changes
routing replaces this section with its own run, so `git log -p` on this file
shows the tally moving from red to green.

```
== Results
PHASE  RESULT   NAME
1      PASS     gh api user returns the authenticated login
1      PASS     gh api repos/cli/cli returns real repository data
1      PASS     gh api graphql returns the authenticated login
1      PASS     gh repo view returns real repository data
1      PASS     gh api --paginate followed the rewritten Link header (82 labels)
1      PASS     gateway recorded the authenticated REST request for /user
1      PASS     gateway recorded the authenticated REST request for the repository
1      PASS     gateway recorded the authenticated GraphQL requests
1      PASS     gateway recorded the second page of labels
1      PASS     second page request carried the token
2      PASS     gh api user still works without an override
2      PASS     gh api repos/cli/cli still works without an override
2      PASS     gh api graphql still works without an override
2      PASS     gh repo view still works without an override
2      PASS     gateway saw no traffic without an override
3      PASS     gh cannot reach GitHub directly while blackholed
4      PASS     basic-rest.txtar
4      PASS     basic-graphql.txtar
4      FAIL     release-upload-download.txtar
4      FAIL     repo-delete.txtar
4      FAIL     repo-list-rename.txtar
4      FAIL     repo-read-file.txtar
4      FAIL     repo-rename-transfer-ownership.txtar
4      FAIL     run-download.txtar
4      FAIL     extension.txtar
4      FAIL     search-issues.txtar
4      FAIL     auth-status.txtar
5      PASS     gist-create-view-delete.txtar

== Summary
9 subset script(s) red: release-upload-download.txtar repo-delete.txtar repo-list-rename.txtar repo-read-file.txtar repo-rename-transfer-ownership.txtar run-download.txtar extension.txtar search-issues.txtar auth-status.txtar
```
