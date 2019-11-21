% GH(1) Version 0.0.0 | GitHub on the CLI Documentation

NAME
====

**gh** — Interact with GitHub from the CLI

SYNOPSIS
========

| **gh** \[**global options**...] \[**command**] \[**command options**...]
| **gh** \[**-R|--repo** _owner/repo_|**-B|--current-branch** _branch_] \[**pr|issue|help**] \[**command options**...]
| **gh** \[**-h**|**--help**] 


DESCRIPTION
===========

Do thinks with GitHub using your terminal.

gh is intended to be run while currently in a git repository that has a GitHub
remote. An initial authentication wizard will guide you on first run to connect
gh to your GitHub account.

Interact with GitHub objects via subcommands like _gh issue create_ or _gh pr
status_.

Global Options
--------------

-h, --help

:   Prints brief usage information.

-R, --repo "owner/name"

:   Select a repo other than the current directory to work with

-B, --current-branch "branch"

:   Select a branch to work from other than the currently checked out branch

Commands
--------

pr checkout "pr-number"

:   Locally check out a given Pull Request

pr create

pr create --draft

pr create --title "a title" --body "short body"

: Create a PR. Will prompt you for title and body if not provided. Uses $EDITOR
or $VISUAL to open PR body for editing. Pushes branch if not already pushed and
warns about uncommited changes.

TODO add rest of commands / more examples

FILES
=====

*~/.config/gh*

:   Config file and auth storage for gh

ENVIRONMENT
===========

TODO

BUGS
====

See GitHub Issues: <https://github.com/github/gh-cli/issues>

AUTHOR
======

GitHub <https://github.com/github>

SEE ALSO
========

**git(1)**

