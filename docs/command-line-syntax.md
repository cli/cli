# How we document our command line syntax

## Required arguments

Use plain text

_*example:*_
`gh help`
help is a required argument in this command

## Placeholder values

Use Angled brackets to represent text that you must supply

_*example:*_
`gh pr view <issueNumber>`
Replace `<issueNumber>` with an issue number

## Optional arguments

Place optional arguments in square brackets

_*example:*_
`gh pr checkout [--web]`
Replace `--web` is an optional argument

## Mutually exclusive arguments

Place mutually exclusive arguments inside braces separated by a vertical bar.

_*example:*_
`gh pr {view | create}`

## Repeatable arguements

One or more arguments can replace arguments with ellipsis

_*example:*_
`gh pr close <numbers … >`
