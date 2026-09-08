# Testing

[Development guides](README.md)

Guidance for validating changes and writing or reviewing tests, fixtures, and
generated test doubles.

## Local validation

Run from the repository root with the toolchain declared in [go.mod](../go.mod).
For building and exercising the local binary, see
[CONTRIBUTING](../.github/CONTRIBUTING.md#building-the-project).

Start with affected-package tests; substitute the actual package and test name
in these examples:

```sh
go test ./pkg/cmd/issue/list/... -run '^TestNewCmdList$'
go test ./pkg/cmd/issue/list/...
```

Before committing code changes, run the existing quality gates:

```sh
go fix ./pkg/cmd/issue/list/...  # Substitute changed packages; inspect its edits
go test ./...
make lint                      # golangci-lint run ./...
```

Report blocked checks honestly. These checks are not full CI parity; see
[CI and live acceptance tests](#ci-and-live-acceptance-tests-are-different) below.
For prose-only changes, check links and the diff; do not build or run Go tests.

## Choose the behavioral seam

Command suites generally separate constructor tests from run-function tests.
Use table-driven tests for variations of the same behavior, not a mandatory
single table spanning unrelated behaviors.

| Changed behavior | Test surface |
| --- | --- |
| Flag parsing, defaults, validation, Options curation | Constructor with injected `runF`; see `TestNewCmdList` in [issue/list tests](../pkg/cmd/issue/list/list_test.go) |
| Business behavior, output, HTTP/Git interactions | Run function or the command's existing execution seam |
| Repository overrides and pre-run wiring | Command execution with the relevant parent hooks and override |
| TTY-sensitive output or prompts | Relevant TTY/non-TTY cases, with separate stream assertions |
| JSON output | Exported data contract, including empty results |
| Parsing/mapping input variations | Small table test; see `TestParseAgentName` in [agent detection tests](../internal/agents/detect_test.go) |

Prove changed behavior, including relevant error paths and regressions; do not
repeat the same assertion at every layer. The named examples demonstrate those
seams, not every helper in their files. The top-level `test` package is
[deprecated](project-layout.md); do not copy legacy `test.ExpectLines` or
command harnesses into new tests. Prefer exact assertions for stable output.

Use `testify`: **use `require`, not `assert`, for error checks**, so an
unexpected error stops the test before dependent assertions. Use
`require.NoError`, `require.Error`, or the appropriate `require` error matcher;
ordinary value comparisons can use `assert.Equal`.

## Isolate effects and observe streams

Ordinary tests must not use the operator's GitHub account, home/configuration,
keyring, repositories, or live network. Use the existing command seams, fake
configuration, test-owned temporary directories, and HTTP/Git doubles. Real
filesystem or Git operations must stay within a test-owned environment.

[`iostreams.Test`](../pkg/iostreams/iostreams.go) returns
`ios, stdin, stdout, stderr`, with TTY flags initially false. Set
`SetStdinTTY`, `SetStdoutTTY`, and `SetStderrTTY` explicitly for the behavior
under test; stdout being a terminal does not imply stdin is interactive.
Assert stdout and stderr separately, including the stream expected to be empty.
Keep time, terminal width, color, and prompt answers deterministic where relevant.

## HTTP doubles and generated mocks

Use [`httpmock.Registry`](../pkg/httpmock/registry.go) as the injected
`http.Client` transport. Register expected requests and always arrange
`defer reg.Verify(t)` so unused stubs fail the test:

```go
reg := &httpmock.Registry{}
defer reg.Verify(t)
reg.Register(
    httpmock.REST("GET", "repos/OWNER/REPO"),
    httpmock.JSONResponse(map[string]any{"name": "REPO"}),
)
client := &http.Client{Transport: reg}
```

Use that client through the command's HTTP dependency. Matchers/responders in
[stub.go](../pkg/httpmock/stub.go) include `REST`, `GraphQL`, `JSONResponse`,
and `FileResponse`. Verify payloads or query variables when their values are the
behavior being changed. Host-selection tests must also assert the request host:
the ordinary `REST` and `GraphQL` matchers do not check it.

Before changing an interface with generated mocks, find its `go:generate`
directive and generated consumers. Use the declared generator in affected
packages; do not hand-edit generated mocks or run `go generate ./...` by default.
For example, [Prompter](../internal/prompter/prompter.go) declares `moq`;
when that package is affected, run `go generate ./internal/prompter` from the
root and inspect the generated diff. Broaden only for actual dependent mocks.

## CI and live acceptance tests are different

`make lint` runs `golangci-lint run ./...`, not all CI checks.
[Lint CI](../.github/workflows/lint.yml) also checks `go mod tidy -diff`,
`go fix -diff ./...`, license generation, and vulnerabilities.
[Go CI](../.github/workflows/go.yml) runs `go test -race -tags=integration ./...`
on Linux, Windows, and macOS, plus a separate attestation integration job with
its own environment. Inspect the relevant workflow/setup before reproducing
environment-dependent checks; an ordinary test run is not evidence of CI parity.

**Before running or changing acceptance tests**, read the
[Acceptance README](../acceptance/README.md). They create/manage real GitHub
resources and are not ordinary unit tests or the `integration` tag. Running
them requires explicitly authorized `GH_ACCEPTANCE_HOST`, `GH_ACCEPTANCE_ORG`,
and `GH_ACCEPTANCE_TOKEN`, including permission to clean up. Do not borrow the
operator's token or treat available credentials as permission. Preserve resource
cleanup; verbose logs can expose environment secrets despite redaction.
