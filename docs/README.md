# GitHub CLI Documentation

This is the [GitHub CLI](https://github.com/gh-cli) development
documentation.

## Usage

Setup:

- Install: `brew install github/gh/gh --HEAD`
- Upgrade: `brew upgrade gh --fetch-HEAD`

Commands:

- `gh`                      
  - `—version`
- `gh help`
- `gh pr create`
  - `-d`, `--draft` Mark PR as a draft         
  - `-t`, `--title` string, Supply a title. Will prompt for one otherwise.
  - `-b`, `--body` string, Supply a body. Will prompt for one otherwise
  - `-P`, `--no-push`
  - `-I`, `--noninteractive`
  - `-T`, `--target` string, The branch into which you want your code merged
- `gh pr list`
  - `-s, --state` string, filter by state (open|closed|all)
  - `-L, --limit` int, maximum number of issues to fetch (default 30)
  - `-b`, `--body` string, Supply a body. Will prompt for one otherwise
  - `-l, --label` strings, filter by label
- `gh pr view [number]`
- `gh pr status`
- `gh pr checkout [number]`
- `gh issue create`
  - `-R/--repo`
  - `-m, --message` stringArray, set title and body
  - `-w, --web` open the web browser to create an issue
- `gh issue list`
  - `-a, --assignee` string, filter by assignee
  - `-l, --label` strings, filter by label
  - `-L, --limit` int, maximum number of issues to fetch (default 30)
  - `-s, --state` string, filter by state (open|closed|all)
- `gh issue view [number]`
- `gh status`
- `gh push`
- `gh add`
- `gh commit`

Global Flags:

- `-B`, `--current-branch string`
- `-R`, `--repo string`
- `-h`, `--help`


