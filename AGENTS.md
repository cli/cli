# AGENTS.md

This is the GitHub CLI (`gh`), a command-line tool for interacting with GitHub. The module path is `github.com/cli/cli/v2`.

## Build, Test, and Lint

```bash
make                                       # Build (Unix) — outputs bin/gh
go run script/build.go                     # Build (Windows)
go test ./...                              # All unit tests
go test ./pkg/cmd/issue/list/... -run TestIssueList_nontty  # Single test
go test -tags acceptance ./acceptance      # Acceptance tests
make lint                                  # golangci-lint (same as CI)
```

## Architecture

Entry point: `cmd/gh/main.go` → `internal/ghcmd.Main()` → `pkg/cmd/root.NewCmdRoot()`.

Key packages:
- `pkg/cmd/<command>/<subcommand>/` — CLI command implementations
- `pkg/cmdutil/` — Factory, error types, flag helpers (`NilStringFlag`, `NilBoolFlag`, `StringEnumFlag`)
- `pkg/iostreams/` — I/O abstraction with TTY detection, color, pager
- `pkg/httpmock/` — HTTP mocking for tests
- `api/` — GitHub API client (GraphQL + REST)
- `internal/featuredetection/` — GitHub.com vs GHES capability detection
- `internal/tableprinter/` — Table output for list commands

## Command Structure

A command `gh foo bar` lives in `pkg/cmd/foo/bar/` with `bar.go`, `bar_test.go`, and optionally `http.go`/`http_test.go`.

### Canonical Examples

- **Command + tests**: `pkg/cmd/issue/list/list.go` and `list_test.go`
- **Factory wiring**: `pkg/cmd/factory/default.go`
- **Unit tests**: `internal/agents/detect_test.go`

### The Options + Factory Pattern

Every command follows this structure (see `pkg/cmd/issue/list/list.go`):

1. `Options` struct with `IO`, `HttpClient`, `Config`, `BaseRepo` + flags
2. `NewCmdFoo(f *cmdutil.Factory, runF func(*FooOptions) error)` constructor — `runF` is the test injection point
3. Separate `fooRun(opts)` function with the business logic

Key rules:
- Lazy-init `BaseRepo`, `Remotes`, `Branch` inside `RunE`, not the constructor
- Commands register in `pkg/cmd/root/root.go`; subcommand groups use `cmdutil.AddGroup()`

### Command Examples and Help Text

Use `heredoc.Doc` for examples with `#` comment lines and `$ ` command prefixes:
```go
Example: heredoc.Doc(`
    # Do the thing
    $ gh foo bar --flag value
`),
```

### JSON Output

Add `--json`, `--jq`, `--template` flags via `cmdutil.AddJSONFlags(cmd, &opts.Exporter, fieldNames)`. In the run function: `if opts.Exporter != nil { return opts.Exporter.Write(opts.IO, data) }`. See `pkg/cmd/pr/list/list.go`.

## Testing

### HTTP Mocking

Use `httpmock.Registry` with `defer reg.Verify(t)` to ensure all stubs are called:

```go
reg := &httpmock.Registry{}
defer reg.Verify(t)

reg.Register(
    httpmock.REST("GET", "repos/OWNER/REPO"),
    httpmock.JSONResponse(someData),
)
reg.Register(
    httpmock.GraphQL(`query PullRequestList\b`),
    httpmock.FileResponse("./fixtures/prList.json"),
)
client := &http.Client{Transport: reg}
```

Common: `REST(method, path)`, `GraphQL(pattern)`, `JSONResponse(body)`, `FileResponse(path)`. See `pkg/httpmock/` for all matchers/responders.

### IOStreams in Tests

```go
ios, stdin, stdout, stderr := iostreams.Test()
ios.SetStdoutTTY(true)  // simulate terminal
```

### Assertions

Use `testify`. Always use `require` (not `assert`) for error checks so the test halts immediately:

```go
require.NoError(t, err)
require.Error(t, err)
assert.Equal(t, "expected", actual)
```

### Generated Mocks

Interfaces use `moq`: `//go:generate moq -rm -out prompter_mock.go . Prompter`. Run `go generate ./...` after interface changes.

### Table-Driven Tests

Use table-driven tests for functions with multiple input/output scenarios. See `internal/agents/detect_test.go` or `pkg/cmd/issue/list/list_test.go` for examples:

```go
tests := []struct {
    name      string
    // inputs and expected outputs
}{
    {name: "descriptive case name", ...},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // arrange, act, assert
    })
}
```

## Code Style

- Add godoc comments to all exported functions, types, and constants
- Avoid unnecessary code comments — only comment when the *why* isn't obvious from the code
- Do not comment just to restate what the code does

## Error Handling

Error types in `pkg/cmdutil/errors.go`:
- `FlagErrorf(...)` — flag validation (prints usage)
- `cmdutil.SilentError` — exit 1, no message
- `cmdutil.CancelError` — user cancelled
- `cmdutil.PendingError` — outcome pending
- `cmdutil.NoResultsError` — empty results

Use `cmdutil.MutuallyExclusive("message", cond1, cond2)` for mutually exclusive flags.

## Feature Detection

Commands using feature detection must include a `// TODO <cleanupIdentifier>` comment directly above the if-statement for linter compliance:

```go
// TODO someFeatureCleanup
if features.SomeCapability {
    // use new API
} else {
    // fallback for older GHES
}
```

## API Patterns

```go
client := api.NewClientFromHTTP(httpClient)
client.GraphQL(hostname, query, variables, &data)
client.REST(hostname, "GET", "repos/owner/repo", nil, &data)
```

For host resolution, use `cfg.Authentication().DefaultHost()` — not `ghinstance.Default()` which always returns `github.com`.