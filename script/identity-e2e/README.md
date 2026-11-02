## Credential-free regression journeys

Go tests provide offline regression journeys. Build the prerelease first and pass its absolute path:

```sh
make
go test ./script/identity-e2e -run '^TestOffline' -count=1 -args -gh "$PWD/bin/gh"
go test ./script/identity-e2e -run '^TestOfflineScopes$' -v -count=1 -args -gh "$PWD/bin/gh" -keep
```

`offline_test.go` uses fake accounts, simulated HTTPS/SSH transports, and no GitHub
resources. It runs real `git credential fill`, commits, and clones of empty
simulated repositories, not libcurl or real key authentication. Without `-gh`,
these journeys skip rather than using an installed executable. They require
macOS or Linux.

With `-keep` and verbose test output, each journey prints a fixture `enter.sh` to source in a disposable
subshell for offline exploration. HOME, gh config, Git helpers, and SSH are
redirected into the fixture; `.invalid` hosts and tokens are fake. Do not
replace those hosts with live ones. Each case is independent; use Go's `-run`
regular expression to select journeys.

For reset/downgrade verification, use copies of a stable binary with fake credentials:

```sh
go test ./script/identity-e2e -run '^TestResetDowngrade$' -count=1 \
  -args -gh "$PWD/bin/gh" -old-gh /absolute/path/to/old/gh
```

The harness does not replace the installed executable or access live GitHub resources.
