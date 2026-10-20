Prefix: GHCL
Next: 7

### GHCL-5 · P3 · Read remote details with fewer Git processes

- **Area:** Performance
- **Where:** `git/client.go:165`
- **Problem:** `Remotes` runs `git remote -v` and then a separate `git config --get-regexp` to read resolved remote values. Repository-aware commands pay for both processes during remote discovery.
- **Recommendation:** Consolidate remote URL and resolved-value reads into fewer Git invocations while preserving URL rewrites, fetch and push URLs, and empty-config behavior.
- **Fix with:** make-it-fast
- **Effort:** M
