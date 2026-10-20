Prefix: GHCL
Next: 7

### GHCL-1 · P2 · Let commands that do not use config start without it

- **Area:** Config
- **Where:** `internal/ghcmd/cmd.go:53`, `pkg/cmd/root/root.go:57`, `pkg/cmdutil/factory.go:27`
- **Problem:** `Main` loads configuration before dispatch, and `NewCmdRoot` returns an error when `f.Config()` fails. A malformed or unreadable config therefore blocks commands such as `gh version` that do not need config. Every invocation also pays the config load cost.
- **Recommendation:** Resolve config only for the selected command paths that need it. Preserve config-backed aliases and authentication checks, and define which config errors those paths report.
- **Fix with:** make-it-fast
- **Effort:** M

### GHCL-2 · P2 · Skip unused GraphQL preloads for piped PR views

- **Area:** Performance
- **Where:** `pkg/cmd/pr/view/view.go:92`, `pkg/cmd/pr/shared/finder.go:266`
- **Problem:** The default field set includes `comments` and `statusCheckRollup`, which make the finder send separate preload queries. `viewRun` checks whether stdout is a terminal only after the finder returns. The default piped preview without `--comments` does not display comments or checks. The latency impact is unproven.
- **Recommendation:** Choose the field set before calling the finder. Omit data the default piped preview does not render, while preserving the richer terminal view, `--comments`, and `--json` behavior.
- **Fix with:** make-it-fast
- **Effort:** S

### GHCL-3 · P3 · Build only the selected command family for normal invocations

- **Area:** Performance
- **Where:** `pkg/cmd/root/root.go:117`
- **Problem:** `NewCmdRoot` calls every top-level command constructor before Cobra dispatches the requested command. The lazy reference change avoids building reference text, but it does not avoid constructing unrelated command families. The startup cost is unmeasured.
- **Recommendation:** Measure command construction, then route normal invocations to the selected family if the cost matters. Keep full-tree behavior for root help, reference output, completion, and configured aliases. Preserve Cobra flags, help, and error behavior.
- **Fix with:** make-it-fast
- **Effort:** L
- **Design:** [GHCL-3 lazy command routing](docs/rfc/GHCL-3-lazy-command-routing.md)

### GHCL-4 · P3 · Reuse the configured HTTP client within one invocation

- **Area:** Performance
- **Where:** `pkg/cmd/factory/default.go:147`, `pkg/cmd/factory/default.go:183`, `pkg/cmd/pr/list/list.go:146`
- **Problem:** Each call to the configured `HttpClient` factory constructs a new client. `pr list` creates one before resolving its base repository, and the smart base repository resolver creates another client in the same invocation.
- **Recommendation:** Reuse the configured client within one invocation. Define how config or authentication changes affect that client, and leave the plain and external client behavior intact.
- **Fix with:** make-it-fast
- **Effort:** S

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
