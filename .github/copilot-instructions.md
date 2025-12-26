# GitHub CLI - AI Agent Instructions

GitHub CLI (`gh`) is a command-line interface for GitHub that brings PRs, issues, and GitHub concepts to the terminal. This is a Go project with a modular command architecture.

## Architecture Overview

**Directory Structure:**
- `cmd/gh/main.go` - Entry point; calls `ghcmd.Main()` which orchestrates the CLI
- `pkg/cmd/<domain>/<subcommand>/` - Individual command implementations (e.g., `pkg/cmd/pr/create/`)
- `pkg/cmdutil/` - Shared command utilities: `Factory` (dependency injector), `IOStreams`, flag helpers
- `api/` - GitHub API client wrapper (GraphQL + REST)
- `internal/gh*` - Core domain packages: `ghrepo` (repository references), `ghinstance` (hostname handling), `config` (auth/settings)
- `git/` - Git repository utilities
- `pkg/extensions/` - Third-party command extension system

**Command Execution Flow:**
1. `cmd/gh/main.go` → `ghcmd.Main()` (exit code handler)
2. Root command (`pkg/cmd/root/root.go`) → dispatcher via cobra
3. Domain command (`pkg/cmd/issue/list/list.go`) → `NewCmdXxx()` returns `*cobra.Command`
4. Logic runs in command's `RunE` handler → writes to `opts.IO` streams

## Key Conventions & Patterns

**Command Structure (Required for all new commands):**
- Each command is a package exposing `NewCmdXxx(f *cmdutil.Factory, runF func(*XxxOptions) error) *cobra.Command`
- Options struct stores flags, factory functions, and state (e.g., `CreateOptions` in [pr/create/create.go](pkg/cmd/pr/create/create.go#L30))
- Separation: `NewCmdXxx()` = flag setup; `runF()` = execution logic
- Use `cmdutil.EnableRepoOverride(cmd, f)` to support `--repo` flag for targeting repos

**Testing Approach:**
- Mock HTTP requests with `httpmock.Registry` (see [api/client_test.go](api/client_test.go#L20))
- Use `cmdutil.Factory` injection rather than real Git/API calls
- Stub prompters, git operations, API responses for isolated unit tests
- Reference: Test functions take `runF` callback instead of directly implementing (see [pkg/jsonfieldstest](pkg/jsonfieldstest/jsonfieldstest.go#L32))

**Build & Run:**
- `make bin/gh` - Builds to `bin/gh` binary via `script/build.go`
- `make test` - Runs Go tests across all packages
- `make acceptance` - Runs acceptance tests with `-tags acceptance` flag
- `go run script/build.go clean` - Removes build artifacts

**Error Handling:**
- Special exit codes: `exitError=1`, `exitCancel=2`, `exitAuth=4`, `exitPending=8` (see [ghcmd/cmd.go](internal/ghcmd/cmd.go#L35))
- Use `cmdutil.SilentError` for user-facing errors (no stack trace)
- Wraps auth/HTTP/cancellation errors distinctly for appropriate exit codes

**IO Streams (Output):**
- All output via `opts.IO` (`*iostreams.IOStreams`) with `.Out`, `.ErrOut`, `.ColorEnabled()`
- Never use `fmt.Print*` or `log.*` directly
- Use `pager.Pager` for long output (respects `GH_PAGER` env var)

**Flag Patterns:**
- Repository override: `--repo owner/repo`
- JSON output: `--json` with field selection (tested via [pkg/jsonfieldstest](pkg/jsonfieldstest/jsonfieldstest.go))
- Boolean flags use proper Cobra naming: `cmd.Flags().Bool("draft", false, "...")` → accessed as `opts.IsDraft`

## Critical Dependencies & Integration Points

**GitHub API Access:**
- `f.HttpClient()` returns authenticated `*http.Client` (or `f.PlainHttpClient()` for custom headers)
- `api.Client` wraps HTTP calls; provides `GraphQL()` and `REST()` methods
- GraphQL queries in `api/queries_*.go` files
- Hostname handling via `ghinstance.NormalizeHostname()` for Enterprise support

**Git & Repo Resolution:**
- `f.BaseRepo()` → current repo (`ghrepo.Interface`)
- `f.Remotes()` → git remotes from local repo
- `f.Branch()` → current branch name
- `git.Client` for operations like `IsAncestor()`, `AddFiles()` (see [git/client.go](git/client.go))

**Config & Auth:**
- `f.Config()` returns `gh.Config` (handles multi-account, oauth tokens, git credentials)
- Auth check via `cmdutil.DisableAuthCheck(cmd)` for commands not requiring authentication
- Stored in `~/.config/gh/` (Linux/macOS) or `%APPDATA%\GitHub CLI` (Windows)

## Project-Specific Workflows

**Adding a New Command:**
1. Create `pkg/cmd/<domain>/<action>/` package
2. Implement `NewCmd<Action>(f *cmdutil.Factory, runF func(*<Action>Options) error) *cobra.Command`
3. Register in parent domain command using `cmdutil.AddGroup(cmd, title, newCmd())`
4. Always include help text with `Example` field and Cobra annotations for arguments

**Multi-Domain Commands (Issue/PR):**
- Shared logic in `pkg/cmd/<domain>/shared/` (e.g., `pr/shared` for common PR utilities)
- `pr/shared.PRFinder`, `pr/shared.QualifiedHeadRef` - reusable interfaces

**Extending via Extensions:**
- User-defined commands in `~/.config/gh/extensions/`
- Invoked as `gh <name>` if not a built-in command
- Exit codes propagated via `ExternalCommandExitError`

## Common Pitfalls to Avoid

- **Never use `os.Getenv()` directly** - use `f.Config()` for settings, respect env var namespacing (`GH_*`)
- **Don't mock time.Now()** - inject time via options if needed (prefer fixed timestamps in tests)
- **Avoid global state** - pass dependencies via `Factory` injection
- **No real HTTP calls in tests** - always use `httpmock.Registry`
- **Never write to user's filesystem outside temp locations** - respect `GH_CONFIG_DIR` for config paths
