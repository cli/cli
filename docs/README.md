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
  - `-d`, `--draft`         
  - `-t`, `--title string`
  - `-b`, `--body string`
  - `-P`, `--no-push`
  - `-I`, `--noninteractive`
  - `-T`, `--target string`
  - `--title`
  - `--body`
- `gh pr list`
  - `--state`
  - `--limit`
- `gh pr view [number]`
- `gh pr status`
- `gh pr checkout [number]`
- `gh issue create`
  - `-R/--repo`
  - `--message`
  - `--web`
- `gh issue list`
  - `-l`
- `gh issue status`
- `gh issue view`
- `gh status`
- `gh push`
- `gh add`
- `gh commit`

Global Flags:

- `-B`, `--current-branch string`
- `-R`, `--repo string`
- `-h`, `--help`


