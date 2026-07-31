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
`gh` was sent back to the gateway rather than to `api.github.com`.

**Phase 2, control.** No `api_host` and no blackhole. The same commands must
still work and the gateway must record nothing, so the override is what causes
the routing.

**Phase 3, blackhole sanity.** No `api_host`, blackhole back on. `gh api user`
must fail. Without this, phase 1 could pass through a direct connection that the
blackhole was silently failing to prevent.

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

## Expected result today
Phase 1 fails and phases 2 and 3 pass. `gh` builds its own REST and GraphQL
endpoints in `internal/ghinstance` and passes `Host: "none"` to go-gh's
`NewHTTPClient`, so it never consults `api_host` and cannot reach the blackholed
canonical host. That red result is the point: it is the acceptance criterion for
teaching `gh` itself about `api_host`.
