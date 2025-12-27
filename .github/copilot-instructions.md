# GitHub CLI - AI Agent Instructions

GitHub CLI (`gh`) is a command-line interface for GitHub that brings PRs, issues, and GitHub concepts to the terminal. This is a Go project with a modular command architecture.

## Architecture Overview

**Directory Structure:**
- `cmd/gh/main.go` - Entry point; calls `ghcmd.Main()` which orchestrates the CLI
- `pkg/cmd/<domain>/<subcommand>/` - Individual command implementations (e.g., `pkg/cmd/pr/create/`)
  - Domains: `pr`, `issue`, `repo`, `gist`, `actions`, `workflow`, `codespace`, `auth`, `extension`, `config`, etc.
  - Each subcommand is a separate `.go` file (e.g., `create.go`, `list.go`) with its own `NewCmd*()` function
- `pkg/cmdutil/` - Shared command utilities: `Factory` (dependency injector), `IOStreams`, flag helpers
- `api/` - GitHub API client wrapper (GraphQL + REST); `queries_*.go` files contain GraphQL queries
- `internal/gh*` - Core domain packages: `ghrepo` (repo references), `ghinstance` (hostname), `config` (auth/settings), `ghcmd` (exit codes)
- `git/` - Git repository utilities
- `pkg/extensions/` - Third-party command extension system
- `docs/project-layout.md` - Definitive architecture documentation

**Command Execution Flow:**
1. `cmd/gh/main.go` → `ghcmd.Main()` returns exit code
2. `ghcmd.Main()` → creates `Factory` and loads root command
3. Root command (`pkg/cmd/root/root.go`) → dispatches to domain/subcommand via Cobra
4. Subcommand (e.g., `pkg/cmd/pr/create/create.go`) → `NewCmdCreate()` defines flags/help, `RunE` calls `createRun()`
5. `createRun()` uses injected factory functions to perform work, writes to `opts.IO` streams
6. Exit with appropriate exit code (0=success, 1=error, 2=cancel, 4=auth, 8=pending)

## Key Conventions & Patterns

**Command Structure (Required for all new commands):**
- Each command is a package exposing `NewCmdXxx(f *cmdutil.Factory, runF func(*XxxOptions) error) *cobra.Command`
- Options struct stores flags, factory functions, and state (e.g., `CreateOptions` in [pkg/cmd/pr/create/create.go](pkg/cmd/pr/create/create.go#L30))
- Separation of concerns: `NewCmdXxx()` = flag setup & help; `runF()` = execution logic
- Use `cmdutil.EnableRepoOverride(cmd, f)` to support `--repo` flag for targeting repos
- Use `cmd.SetContext()` to pass contexts through command hierarchy

**Testing Approach (Critical - See [pkg/cmd/issue/list/list_test.go](pkg/cmd/issue/list/list_test.go)):**
- Mock HTTP requests with `httpmock.Registry` and `.Register()` with `httpmock.GraphQL()` or `httpmock.REST()`
- Stub all Factory methods: `HttpClient`, `Config`, `BaseRepo`, `Branch`, `Remotes`, etc.
- Use `iostreams.Test()` to capture output streams without real TTY
- Test pattern: `runCommand(httpMock, isTTY, cliArgs)` → parse response fixtures → assert output
- Inject `opts.Now()` func for reproducible time-based tests
- Response fixtures in `./fixtures/` directories (JSON files)
- Never shell out to real git/curl; never make real API calls
- Table-driven tests for variations

**Build & Run:**
- `make bin/gh` - Builds binary via `script/build.go`
- `make test` - Runs all Go tests
- `make acceptance` - Runs acceptance tests with `-tags acceptance` flag
- `go run script/build.go clean` - Removes artifacts
- Binary is platform-specific (Windows needs `.exe`)

**Error Handling:**
- Special exit codes in `internal/ghcmd/cmd.go`: `exitError=1`, `exitCancel=2`, `exitAuth=4`, `exitPending=8`
- Use `cmdutil.SilentError` for user-facing errors (suppresses stack trace)
- Wrap auth/HTTP/cancellation errors distinctly for appropriate exit codes

**IO Streams (Output):**
- All output via `opts.IO` (`*iostreams.IOStreams`) with `.Out`, `.ErrOut`, `.ColorEnabled()`
- Never use `fmt.Print*` or `log.*` directly
- Use `pager.Pager` for long output (respects `GH_PAGER` env var)
- Respect `isTerminal` vs non-TTY output formatting (tables for TTY, plain text for pipes)

**Flag Patterns:**
- Repository override: `--repo owner/repo` (use `cmdutil.EnableRepoOverride()`)
- JSON output: `--json` with field selection via Cobra annotations
- Boolean flags: `cmd.Flags().Bool("draft", false, "...")` → accessed as `opts.IsDraft`
- Use `cmd.Flags().StringArrayP()` for repeatable flags (e.g., `--assignee user1 --assignee user2`)

## Critical Dependencies & Integration Points

**GitHub API Access:**
- `f.HttpClient()` returns authenticated `*http.Client` (auto-includes auth headers)
- `f.PlainHttpClient()` for custom header control (e.g., during login)
- `api.Client` created via `api.NewClientFromHTTP(httpClient)`
- GraphQL: `client.Query(hostname, name, queryStruct, vars)`, `client.Mutate()`, `client.GraphQL()`
- REST: `client.REST(hostname, method, path, body, headers)` - see [api/queries_*.go](api/) for query definitions
- Hostname handling via `ghinstance.NormalizeHostname()` for Enterprise support
- Auth scopes determined via config; use `cmdutil.DisableAuthCheck(cmd)` for unauthenticated commands

**Git & Repo Resolution:**
- `f.BaseRepo()` → current repo (`ghrepo.Interface` with `.Owner()`, `.Name()` methods)
- `f.Remotes()` → parse git remotes from local repo
- `f.Branch()` → current branch name
- `git.Client` for operations: `IsAncestor()`, `AddFiles()`, `CommitCreate()`, `GetRemoteURL()`
- `ghrepo.NewWithHost()` for Enterprise support

**Config & Auth:**
- `f.Config()` returns `gh.Config` (multi-account support, oauth tokens, git credentials)
- Config stored in `~/.config/gh/` (Linux/macOS) or `%APPDATA%\GitHub CLI` (Windows)
- Override with `GH_CONFIG_DIR` environment variable
- Auth tokens cached per hostname; Enterprise Server uses different endpoints

**Extensibility (pkg/extensions/):**
- Extensions are user scripts in `~/.config/gh/extensions/`
- Invoked as `gh <name>` if not a built-in command
- Auto-discovered and managed via `gh extension` command

## Project-Specific Workflows

**Adding a New Command:**
1. Create `pkg/cmd/<domain>/<action>/` directory (or add to existing domain)
2. Implement `NewCmd<Action>(f *cmdutil.Factory, runF func(*<Action>Options) error) *cobra.Command`
3. Register in parent command (e.g., `pkg/cmd/<domain>/<domain>.go`) using `cmdutil.AddGroup()`
4. Always include `.Example` and `.Long` help text; use `heredoc.Doc()` for multiline strings
5. Write tests using `httpmock.Registry` pattern

**Shared Logic Across Domains:**
- `pkg/cmd/<domain>/shared/` for domain-specific utilities (e.g., `pr/shared` has `PRFinder`, `QualifiedHeadRef`)
- Cross-domain utilities in `pkg/cmdutil/`
- Avoid putting command logic in global packages

**GraphQL Query Usage:**
- Query structs defined in `api/queries_*.go` (e.g., `api.PullRequest`, `api.IssueFields`)
- Use named queries in mutations: `client.Mutate(hostname, "CreatePullRequest", mutation, vars)`
- Variables are type-safe; GraphQL errors return partial data with error details

**Checking for Updates:**
- Background goroutine in `ghcmd.Main()` checks for updates asynchronously
- Update info passed to command via `opts.HeadRef()` or via config

## Common Pitfalls to Avoid

- **Never use `os.Getenv()` directly** - use `f.Config()` for settings, respect `GH_*` env var namespacing
- **Don't inject time as `time.Now` directly** - pass `func() time.Time` in options for testability
- **Avoid global state** - pass dependencies via `Factory` injection; no package-level vars with side effects
- **No real HTTP calls in tests** - always use `httpmock.Registry` for mocking
- **Never write to filesystem outside temp** - respect `GH_CONFIG_DIR`; use `ioutil.TempDir()` in tests
- **Don't ignore the `isTTY` distinction** - format output differently for terminals vs pipes
- **Avoid shell.Split() for user input** - it's only used in tests; use `args` from flags
