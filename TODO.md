Prefix: GHCL
Next: 7

### GHCL-5 · P3 · Read remote details with fewer Git processes

- **Area:** Performance
- **Where:** `git/client.go:165`
- **Problem:** `Remotes` runs `git remote -v` and then a separate `git config --get-regexp` to read resolved remote values. Repository-aware commands pay for both processes during remote discovery.
- **Recommendation:** Consolidate remote URL and resolved-value reads into fewer Git invocations while preserving URL rewrites, fetch and push URLs, and empty-config behavior.
- **Fix with:** make-it-fast
- **Effort:** M

### GHCL-6 · P4 · Resolve the executable path only when a command needs it

- **Area:** Performance
- **Where:** `internal/ghcmd/cmd.go:67`, `internal/ghcmd/cmd.go:350`, `pkg/cmdutil/factory.go:17`
- **Problem:** `Main` resolves the executable path on every invocation by scanning `PATH` and checking symlinks. Most commands do not use the result. Auth setup commands and an update notice do use it. The startup cost is unmeasured.
- **Recommendation:** Make executable-path resolution lazy for the commands that consume it. Preserve `GH_PATH` overrides and symlink handling.
- **Fix with:** make-it-fast
- **Effort:** S
