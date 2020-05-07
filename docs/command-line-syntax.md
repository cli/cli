# How we document our command line syntax

## Required text

Use plain text for parts of the command that cannot be changed

_example:_
`gh help`
The argument help is required in this command

## Placeholder values

Use angled brackets to represent a value the user must replace

_example:_
`gh pr view <issue-number>`
Replace `<issue-number>` with an issue number

## Optional arguments

Place optional arguments in square brackets

_example:_
`gh pr checkout [--web]`
The argument `--web` is optional.

## Mutually exclusive arguments

Place mutually exclusive arguments inside braces, separate arguments with vertical bars.

_example:_
`gh pr {view | create}`

## Repeatable arguments

Ellipsis represent arguments that can appear multiple times

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
`command sub-command <{path | string}>`

_optional argument with mutually exclusive options:_
`command sub-command [{<path> | <string>}]`
