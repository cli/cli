Prefix: GHCL
Next: 10

### GHCL-7 · P1 · `script/release` deploys `cli/cli`, not this fork

- **Area:** Build
- **Where:** `script/release:82`, `.github/workflows/prerelease.yml:47`, `internal/licenses/licenses.go:25`
- **Problem:** Without `--local`, `script/release` runs `gh workflow -R cli/cli run deployment.yml` with `environment=production` and `dry_run=false`. Proved by running that script with a stub `gh` on `PATH`: the arguments were `workflow -R cli/cli run deployment.yml --ref trunk -f tag_name=v9.9.9-ultrareview-probe -f environment=production -f dry_run=false`. This repository has no `deployment.yml`. The workflow that publishes here is `prerelease` (`workflow_dispatch` only). That build does not run `script/licenses`, so `gh licenses` hits the missing-`report.txt` path and prints the official-build placeholder. Darwin binaries from that workflow are unsigned cross-compiles.
- **Recommendation:** Stop dispatching `cli/cli`. Make `prerelease` on this repository the publish path, and run `script/licenses` for the target `GOOS`/`GOARCH` before `go build`. Keep `script/release --local` as the local GoReleaser build only.
- **Fix with:** none
- **Effort:** M
- **Design:** [docs/rfc/GHCL-7-release-path.md](docs/rfc/GHCL-7-release-path.md)

### GHCL-8 · P2 · `script/run-acceptance` dispatches `cli/cli` acceptance

- **Area:** Tests
- **Where:** `script/run-acceptance:5`, `script/run-acceptance:95`
- **Problem:** The script sets `repository=cli/cli` and `workflow=acceptance.yml`, then calls `gh workflow run`. This tree's workflows are `test.yml`, `prerelease.yml`, and `dependabot-triage-schedule.yml`. Running the script against the real `gh` reached `POST /repos/cli/cli/actions/workflows/354871894/dispatches` and stopped with HTTP 403, admin rights required. That workflow id is cli/cli's active Acceptance Tests workflow (`acceptance.yml` in that repo). No run was created. A token that can dispatch `cli/cli` would start upstream's live acceptance tests. `make acceptance` does not execute the txtar scripts. `TestAcceptance` was removed on purpose and stays removed.
- **Recommendation:** Delete the `cli/cli` dispatch. Do not restore the removed acceptance runner.
- **Fix with:** delete
- **Effort:** S

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
