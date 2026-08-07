# `api_host`

`api_host` sends a host's API traffic somewhere other than that host's usual API
endpoint, so that an organisation can put a gateway in front of GitHub without
every user reconfiguring or re-authenticating.

```yml
# hosts.yml
github.com:
    api_host: gh-gateway.example.com
    users:
        octocat:
            oauth_token: gho_...
```

With that set, `gh api repos/cli/cli` asks `gh-gateway.example.com` rather than
`api.github.com`. Everything else about the host is unchanged: it is still
`github.com` as far as login, git remotes and web URLs are concerned.

## The shape of the problem

`api_host` is a claim about *all* API traffic, so it can only be honoured in one
place. Any code that builds an absolute `https://api.github.com/...` URL and
hands it to an `*http.Client` has already decided where the request goes, and no
amount of configuration downstream can redirect it.

That makes this feature unusual to work on in two ways.

Firstly, it is a property of the whole codebase rather than of any one command,
so it is only as good as its worst call site. Secondly, a bypass is invisible to
ordinary tests: a request that ignores `api_host` and goes to `api.github.com`
succeeds, because `api.github.com` is reachable. Tests pass and the feature is
quietly broken.

The second point is why this feature comes with a purpose-built test that makes
`api.github.com` unreachable, so a bypass fails loudly instead of silently
working. See [`api-host-test-harness.md`](api-host-test-harness.md).

## How it resolves

Request routing lives in [go-gh][go-gh], which reads `api_host` for the host a
request is aimed at and sends the request there instead.

That leaves gh with a problem of its own: credentials. gh resolves tokens from
the hostname in the request URL, and once a request has been redirected that
hostname is the gateway, which gh has never logged in to and holds no token for.
So `api/http_client.go` maps the gateway back to the host it stands in for, via
`HostForAPIHost`, and sends that host's token.

The fallback only ever adds a token where there would have been none. A host gh
is genuinely logged in to keeps resolving to its own token even if some other
host names it as an `api_host`, so the mapping cannot hijack real credentials.

The reverse lookup is deliberately narrow. It answers only "which host does this
hostname stand in for", and says nothing about whether a given piece of code
*should* honour `api_host`. That decision belongs to the caller, because
`api_host` covers API traffic and not, for instance, git operations against the
same host.

## Scope

`api_host` applies to API traffic only. Git operations, browser URLs and OAuth
device flow continue to use the host itself.

It is a per-host setting under `hosts.yml`, not a global one, so a user can
route one host through a gateway and reach another directly.

Configuring the same `api_host` on two hosts is a misconfiguration rather than a
supported topology, since a request arriving at the gateway could belong to
either. The reverse lookup resolves it to the first matching host and does not
report an error.

## Known gaps

`api_host` is not yet honoured centrally. Only the credential half is in place:
a request that reaches a configured gateway is authenticated correctly. Many
call sites still build absolute `api.github.com` URLs and never reach the
gateway at all, and `gh api` does not read `api_host` for the paths it builds.

The test harness enumerates exactly which, and its recorded transcript is the
current tally.

[go-gh]: https://github.com/cli/go-gh
