Prefix: GHCL
Next: 10

### GHCL-5 · P3 · Read remote details with fewer Git processes

- **Area:** Performance
- **Where:** `git/client.go:165`
- **Problem:** `Remotes` runs `git remote -v` and then a separate `git config --get-regexp` to read resolved remote values. Repository-aware commands pay for both processes during remote discovery. Re-checked on this tree: both commands are still issued in `Remotes`.
- **Recommendation:** Consolidate remote URL and resolved-value reads into fewer Git invocations while preserving URL rewrites, fetch and push URLs, and empty-config behavior.
- **Fix with:** make-it-fast
- **Effort:** M

### GHCL-9 · P3 · api-host acceptance phases call a removed test

- **Area:** Tests
- **Where:** `script/api-host-gateway/test.sh:352`, `script/api-host-gateway/test.sh:382`
- **Problem:** With `GH_APIHOST_ACCEPTANCE=yes`, phases 4 and 5 run `go test -run '^TestAcceptance$'`. That test is not in this tree. The same script still requests `extension.txtar`, which is not in this tree. Those phases record NOMATCH. They do not show that the old acceptance scripts honor `api_host`. Phases 1 to 3 still run `gh` directly and do not use that test. The removed runner stays removed.
- **Recommendation:** Delete phases 4 and 5, including the `extension.txtar` call, until this tree has a runner for those scripts.
- **Fix with:** delete
- **Effort:** S
