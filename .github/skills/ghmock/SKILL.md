---
name: ghmock
description: Use when reproducing GitHub CLI behavior with synthetic API, Git or configuration fixtures, comparing CLI builds, testing prompts, or exporting ghmock evidence. Not for live GitHub operations or terminal recordings.
license: MIT
compatibility: Requires ghmock, caller-selected GitHub CLI executables, Git for repository fixtures, and an external sandbox for untrusted commands. Terminal input requires Unix.
---

# Reproduce `gh` behavior with `ghmock`

Run real `gh` executables against controlled test data. Establish what happened
from their output, exit statuses, requests and retained state, not from a
plausible fixture or a successful helper invocation.

## Establish the boundary

`ghmock` routes API and Git traffic but **is not a sandbox**. `gh`, Git,
extensions and aliases can start other programs or bypass proxy settings.
Require an externally enforced network/filesystem sandbox before running
issue-supplied commands. Do not treat the command allowlist as that boundary.

Use synthetic accounts, tokens, repositories and files. Never copy the
operator's configuration, credentials or Git checkout into a fixture.
Configuration fixtures do not isolate an OS keyring.

Use `example.invalid` as the owner of synthetic GitHub repositories, including
their API response URLs. GitHub account names cannot contain periods, so this
namespace cannot refer to a real account. The triage publisher recognizes it as
test data without rewriting captured output or looking up a real repository.
Keep `github.com` as the host. If the reported repository name matters to the
behavior, preserve it; real GitHub references still require visibility checks.

Use a task-owned `GHMOCK_DIR`; the default temporary state directory may be
shared with another investigation. `ghmock reset` deletes all responses, logs,
saved runs and retained Git/configuration state there.

Use the supplied helper and approved `gh` builds. If setup is needed, prepare
them outside the sandbox with the repository's declared Go toolchain:

```sh
work=$(mktemp -d)
go -C script/ghmock build -o "$work/ghmock" .
export PATH="$work:$PATH"
export GHMOCK_DIR="$work/state"
```

These are host-shell preparation commands, not commands to force through a
restricted agent shell. Provide cached schemas through
`GHMOCK_GRAPHQL_SCHEMA` and `GHMOCK_REST_SCHEMA` when the sandbox cannot download
them. Do not disable checks to hide missing schemas.

Run `ghmock --help` and read the complete output before using the helper. In the
issue-triage environment, read `/opt/ghmock/builds.txt` for the executable paths
and versions already verified by preparation. Use that inventory instead of
running version probes; builds absent from it are unavailable for the run.
`source` is the selected checkout, not necessarily the default branch. Record
the actual versions and revisions; do not assume a reported build is available.

## Build the smallest useful API fixture

1. State the reported command, required initial conditions and expected
   observation. Separate the reporter's explanation from the hypothesis being
   tested. Set a finite iteration budget and command timeout.
2. Run the command with an approved executable and inspect its report.
   `ghmock run` exits nonzero for helper failures; the tested `gh` exit status
   is inside the report. Helper success does not mean `gh` succeeded.
3. Use `ghmock log --full` or `--json` to read the actual requests.
   `ghmock scaffold --missing` adds missing responses and requested GraphQL
   fields for the latest completed run.
4. Change only values relevant to the hypothesis with `ghmock set`. Check
   fields with `ghmock schema`. Run the command again after changing the data.
5. Continue until the reported behavior itself is observed, or explain the
   unsupported dependency. An early missing-response error is not a
   reproduction of a later crash.

For example, set `GH_BUILD` to an approved executable, then exercise a known
GraphQL operation:

```sh
ghmock run --gh "$GH_BUILD" -- api graphql \
  -f 'query=query Example { viewer { login } }'
ghmock scaffold --missing
ghmock schema User.login
ghmock set 'graphql Example' --at data.viewer.login --json '"monalisa"'
ghmock run --gh "$GH_BUILD" -- api graphql \
  -f 'query=query Example { viewer { login } }'
ghmock log --full
```

Use request names from the log. REST names are `METHOD /path` on the API host;
other hosts remain in the name. GraphQL names use the selected operation, such
as `graphql Example`. Fixtures are not conditional matches on arbitrary
headers, query parameters or variable values. Use `--append` for successive
responses, or `--index N` to edit one; each build starts from the first response
and repeats the last when it exhausts the sequence.

Scaffolding supplies placeholders, not observed GitHub data. Lists generally
contain one item and pagination stops by default. Configure additional pages
when the behavior needs them. A not-found GraphQL lookup may need both a null
data field and an `errors` entry.

Normal GraphQL responses are trimmed to selected fields. Use `--off-schema`
only when deliberately testing a response outside the schema; state why it is
necessary. Read request and response diagnostics even when `gh` exits zero:
the helper can serve a stored response to a query GitHub would reject.

## Compare builds

Repeat `--gh` to run equivalent conditions through separate executables:

```sh
ghmock run --gh "$REPORTED_BUILD" --gh "$RELEASE_BUILD" --gh "$SOURCE_BUILD" \
  --timeout 30s -- pr view 123 -R example.invalid/hello
```

Identify which build is the baseline. Compare the reported exit statuses,
stdout, stderr and requests. Request differences include GraphQL documents and
variables and REST query strings, not just write bodies. A comparison can show
different behavior without proving why the live service returned particular
data.

## Use real Git fixtures

Prepare remotes, files, commits, branches and linked worktrees with the helper:

```sh
ghmock repo example.invalid/hello
ghmock git -- clone https://github.com/example.invalid/hello.git main
ghmock write main/example.txt --text 'first version'
ghmock git --in main -- add example.txt
ghmock git --in main -- commit -m 'Add example'
ghmock git --in main -- push origin main
ghmock git --in main -- worktree add ../feature -b feature
ghmock run --gh "$GH_BUILD" --in feature -- repo sync
```

`--in` selects a directory in the fixture, not a repository name. `-R` selects
an API repository and is not a substitute for local Git state. API responses
still need the API setup loop; keep their repository URLs and SHAs aligned
with the real fixture.

Each build receives an independent copy of the complete fixture. Inspect the
run's baseline and each retained build, including other linked worktrees:

```sh
ghmock git --run latest --before --in main -- rev-parse HEAD
ghmock git --run latest --build 1 --in main -- status --porcelain=v1
```

Check refs, HEAD content, indexes and working files when they matter, not just
the command's exit status. Without `--run`, `git` and `write` prepare future
runs. Local SSH pack transport does not test SSH keys or host authentication.

## Supply configuration and terminal input

Use `--config-dir DIR` for a prepared synthetic `hosts.yml` and/or `config.yml`.
Each build gets its own writable copy and the original stays unchanged. No
default account or configuration is added. An empty fixture models logged-out
state; do not combine this flag with `--logged-out`. With `--localhost`, name
fixture hosts `github.localhost`.

Use `--stdin TEXT` for piped input. Use `--tty` for color/TTY output without
interactive input. For prompts, use `--terminal` and repeat `--keys` for each
response; supplying keys also enables terminal mode:

```sh
ghmock run --gh "$GH_BUILD" --terminal --keys '{enter}' --timeout 5s -- \
  repo create
```

Keys include `{enter}`, `{tab}`, `{space}`, `{backspace}`, `{esc}`, arrow names,
`{ctrl-c}` and `{ctrl-d}`; `{{` types a brace. Do not combine terminal mode with
`--tty` or `--stdin`. Quiet output does not establish that a prompt is waiting.
Use a short timeout to inspect unanswered prompts. Terminal mode retains raw
bytes and per-key screens; stdout and stderr share the terminal.

## Serve downloads and retain evidence

For an observed, quoted request name, use `set --raw-file FILE` to send exact
bytes or `set --zip-dir DIR` to build a ZIP from prepared synthetic files.
The ZIP helper rejects symlinks. Set the appropriate content type for other
raw formats. JSON logs retain raw bytes as base64; readable output shows byte
counts and SHA-256 instead of printing binary data.

Finish with `ghmock export --archive`. With no path, it uses the caller's
`GHMOCK_EXPORT_PATH`; otherwise provide a new destination file. The archive
retains hidden Git metadata, portable worktree links, responses, raw bytes and
configuration fixtures. Exported configuration includes its fixture tokens.
Keep real credentials out of the state before exporting, not just out of prose.

Treat downloaded archives as untrusted data. Do not execute their hooks or Git
configuration on the operator's machine. Follow the caller's artifact rules;
this skill does not authorize public uploads, comments or labels.

Report the actual `gh` command, captured output, exit status, fixture conditions,
tested builds and limitations. Preserve setup and launcher commands in the raw
evidence. Do not fabricate output, present synthetic data as live GitHub data,
or call a blocked attempt "not reproduced."

## References

- [Full command reference](../../../script/ghmock/main.go)
- [Standalone module and toolchain](../../../script/ghmock/go.mod)
- [Issue-triage reporting contract](../../../docs/issue-triage.md)
