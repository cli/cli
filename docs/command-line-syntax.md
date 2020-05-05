# How we document our command line syntax

## Required arguments

Use plain text

_*example:*_
`gh help`
The argument help is required in this command

## Placeholder values

Use angled brackets to represent a value the user must supply

_*example:*_
`gh pr view <issueNumber>`
Replace `<issue-number>` with an issue number

## Optional arguments

Place optional arguments in square brackets

_*example:*_
`gh pr checkout [--web]`
The argument `--web` is optional.

## Mutually exclusive arguments

Place mutually exclusive arguments inside braces, separate arguments with vertical bars.

_*example:*_
`gh pr {view | create}`

## Repeatable arguements

Ellipsis represent arguments that can appear multiple times

_*example:*_
`gh pr close <numbers … >`

## Variable naming

For multi-word variables use dash-case (all lower case with words seperated by dashes)

_*example:*_
`gh pr checkout <issue-number>`
