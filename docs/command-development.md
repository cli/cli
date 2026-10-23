# Command development

[Development guides](README.md)

Conventions for implementing and reviewing `gh` commands, including command
wiring, flags, prompts, help, and output.

## Structure and lifecycle

A command `gh foo bar` belongs in `pkg/cmd/foo/bar/`, usually with `bar.go`,
`bar_test.go`, and optional `http.go`/`http_test.go`. Keep command-local logic
there; look for a command-family `shared` helper before adding a global API.

For ordinary Cobra commands, follow the existing Options/Factory/run-function
pattern. Include only dependencies the command needs, not a fixed boilerplate
set. Use these named examples for their specific purpose:

| Source symbol | Pattern |
| --- | --- |
| `ListOptions`, `NewCmdList` in [issue/list](../pkg/cmd/issue/list/list.go) | Options hold flags and dependencies; a constructor takes `*cmdutil.Factory` and an injectable `runF` |
| `listRun` in [issue/list](../pkg/cmd/issue/list/list.go) | Run function owns business logic; `RunE` dispatches to `runF` when supplied |
| `New` in [factory/default.go](../pkg/cmd/factory/default.go) | Shared dependency wiring |
| `NewCmdRoot` in [root/root.go](../pkg/cmd/root/root.go) | Top-level registration |
| `NewCmdIssue` in [issue/issue.go](../pkg/cmd/issue/issue.go) | Subcommand registration; `cmdutil.AddGroup` groups help output |

The executable starts at [main](../cmd/gh/main.go), delegates to
[`ghcmd.Main`](../internal/ghcmd/cmd.go), and builds the command tree through
`root.NewCmdRoot`.

Defer repository/Git discovery (`BaseRepo`, `Remotes`, `Branch`) until `RunE` or
the run function, not construction. In particular, **bind
`opts.BaseRepo = f.BaseRepo` inside `RunE`**.
[`EnableRepoOverride`](../pkg/cmdutil/repo_override.go) replaces `f.BaseRepo`
in a pre-run hook; capturing the earlier function can ignore `-R` and `GH_REPO`.
When changing this wiring, exercise execution through the relevant parent hooks,
not just a constructor called in isolation. Use the resolved repository's host
as described in [API and hosts](api-and-hosts.md).

## Flags, help, and errors

Reuse [cmdutil flag helpers](../pkg/cmdutil/flags.go):
`NilStringFlag` and `NilBoolFlag` distinguish omitted values from explicit
empty/false values; `StringEnumFlag` supplies enum validation and completion.
Preserve accepted values, defaults, flag names, and non-interactive paths.

Use `heredoc.Doc` for command examples, with `#` explanatory comment lines and
`$ ` command prefixes. Edit help in command Go source (or
[help_topic.go](../pkg/cmd/root/help_topic.go) for general topics), not
generated manual pages; [CONTRIBUTING](../.github/CONTRIBUTING.md) describes
their release-time generation.

Use [cmdutil errors](../pkg/cmdutil/errors.go), rather than reproducing their
printing or exit handling:

| Helper/type | Meaning |
| --- | --- |
| `FlagErrorf` | Invalid flags/arguments; displays usage |
| `MutuallyExclusive` | Rejects conflicting flag conditions with a flag error |
| `SilentError` | Exit 1 without another message |
| `CancelError` | User cancellation |
| `PendingError` | Outcome pending, not a failure |
| `NoResultsError`, `NewNoResultsError` | Empty results |

## Output and scriptability

Preserve script-facing contracts unless the agreed change explicitly authorizes
breaking them: flags, arguments, defaults, exit behavior, error messages, JSON
fields, non-TTY output, and stdout/stderr routing. Preserve intended TTY behavior
too.

Before changing a terminal interaction, consult [Primer](primer/README.md)
and its [scriptability guidance](primer/foundations/README.md#scriptability).
Use [IOStreams](../pkg/iostreams/iostreams.go) for input/output, TTY detection,
color, and pager handling, and [tableprinter](../internal/tableprinter) for
tables. Keep data on the command's established stdout path and diagnostics on
its stderr path; do not merge streams or leak interactive decoration into pipes.
Non-TTY tables use script-friendly output, not terminal truncation, color, or
headers. Prompts need a non-interactive flag path.

For structured output, use
[`cmdutil.AddJSONFlags`](../pkg/cmdutil/json_flags.go) with the command's
exportable fields to add `--json`, `--jq`, and `--template`. In the run function,
return `opts.Exporter.Write(opts.IO, data)` when the exporter is set, before
human-readable output. See `NewCmdList` and `listRun` above for field selection
and exporter dispatch. Preserve JSON field names, types, and empty-result
behavior independently of terminal rendering.

Before changing tests for these contracts, read [Testing](testing.md).
