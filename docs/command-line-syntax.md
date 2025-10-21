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
Design note: many `gh` commands follow an "all-or-nothing" prompting policy: if any flags or arguments that supply input are provided on the command line, the command will not prompt interactively for other fields. Put another way, commands behave either in a fully interactive mode (no input flags) or in a fully non-interactive mode (all required input supplied via flags/arguments).

Why this exists

- Predictability: automation and scripts can call `gh` with flags and be confident it will not hang waiting for input.
- Simplicity: a single, easy-to-understand model reduces confusion for users — either supply everything up front or be prompted for everything.

Examples

- Interactive (prompts):

  `gh issue create`

  Running without input flags will launch an interactive prompt (or open the editor) to collect required fields such as title and body.

- Non-interactive (no prompts):

  `gh issue create --title "Fix bug" --body "Steps to reproduce..."`

  Supplying any input flags like `--title` or `--body` causes the command to avoid prompting for other fields.

If you expect a prompt but are not prompted

- Ensure no input flags or arguments are present. Even a single flag that supplies a value may disable interactive prompts for the remainder of the command.
- Consult the command's help (for example, `gh help issue create`) to see which flags are considered input flags.

Filing issues

If you think a command should prompt even when specific flags are present (or should behave differently), please open an issue and include:

- The command you ran
- The flags you passed
- What you expected to happen
- What actually happened

See the project's contribution guide for issue templates and guidance: `../.github/CONTRIBUTING.md`.

Rationale for placement

This policy is documented here so users discover it while learning how commands accept input. It's a global UX decision; individual commands should document any exceptions in their own help text.
