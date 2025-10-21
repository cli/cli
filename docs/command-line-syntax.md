# How we document our command line syntax

## Literal text

Use plain text for parts of the command that cannot be changed.

_example:_
`gh help`
The argument help is required in this command.

## Placeholder values

Use angled brackets to represent a value the user must replace. No other expressions can be contained within the angled brackets.

_example:_
`gh pr view <issue-number>`
Replace `<issue-number>` with an issue number.

## Optional arguments

Place optional arguments in square brackets. Mutually exclusive arguments can be included inside square brackets if they are separated with vertical bars.

_example:_
`gh pr checkout [--web]`
The argument `--web` is optional.

`gh pr view [<number> | <url>]`
The `<number>` and `<url>` arguments are optional.

## Required mutually exclusive arguments

Place required mutually exclusive arguments inside braces, separate arguments with vertical bars.

_example:_
`gh pr {view | create}`

## Repeatable arguments

Ellipsis represent arguments that can appear multiple times.

_example:_
`gh pr close <pr-number>...`

## Variable naming

For multi-word variables use dash-case (all lower case with words separated by dashes)

_example:_
`gh pr checkout <issue-number>`

## Additional examples

_optional argument with placeholder:_
`command sub-command [<arg>]`

_required argument with mutually exclusive options:_
`command sub-command {<path> | <string> | literal}`

_optional argument with mutually exclusive options:_
`command sub-command [<path> | <string>]`

## Interactive prompting policy

Design note: many `gh` commands follow an "all-or-nothing" prompting policy. If any flags or arguments that provide input for a command are passed on the command line, the command will not prompt interactively for any additional fields. In other words, commands are intended to be either entirely interactive (no flags for input) or entirely non-interactive (all required input supplied via flags/arguments).

Why this exists
- Predictability: scripts and automation can call `gh` with flags and be confident it will not hang waiting for input.
- Simplicity: ensures a clear mental model for users (either supply everything or be prompted for everything).

Examples
- Interactive (prompts):

	`gh issue create`

	Running without input flags will launch an interactive prompt or editor session to collect required fields such as title and body.

- Non-interactive (no prompts):

	`gh issue create --title "Fix bug" --body "Steps to reproduce..."`

	Supplying any input flags like `--title` or `--body` causes the command to avoid prompting for other fields.

If you expect to be prompted but are not
- Double-check that no input flags or arguments are present. Even a single flag that supplies a value may disable interactive prompts for the remainder of the command.
- See command-specific help (e.g., `gh help issue create`) for which flags are considered input flags.

Filing issues
- If you think a command should prompt even when specific flags are present (or behave differently), please open an issue describing: the command you ran, the flags you passed, the behavior you expected, and the behavior you observed. Link to this policy in the issue to explain the intended default behavior.

Rationale for documentation placement
- This policy is documented here (the command-line syntax docs) so users discover it while learning how commands accept input. It is a global design decision applied to many commands; individual commands should document any exceptions in their own help text.
