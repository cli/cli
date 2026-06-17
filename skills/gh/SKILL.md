---
name: gh
description: Patterns for invoking the GitHub CLI (gh) from agents. Covers structured output, pagination, repo targeting, search vs list, gh api fallback.
---

# Reference

## Interactivity policy

`gh` already does the right thing in non-TTY contexts: it skips the pager,
strips ANSI color, and errors out fast with a helpful message instead of
prompting (e.g. `must provide --title and --body when not running interactively`).
You don't need to defensively set `GH_PAGER` or pass `--no-pager` (no such
flag exists).

## Parsing JSON

Human output from `gh` is column-formatted. If you want structured data:

- Add `--json field1,field2,...` for structured output.
- Run a command with `--json` and **no field list** to print the full set of
  available fields, then pick what you need.
- Use `--jq '<expr>'` for filtering without piping through a separate `jq`.
- Use `--template '<go-template>'` (alongside `--json`) when you want shaped
  text output. Note that `--template`/`-T` collides with a body-template flag
  on a few commands (e.g. `gh pr create -T`, `gh issue create -T`); always
  check `--help` before assuming which one you're hitting.

## Pagination and silent truncation

List commands cap results.

- `gh issue list`, `gh pr list`, `gh search ...`: pass `-L N` (`--limit N`).
  The default is usually 30.
- `gh issue list` / `gh pr list` do not expose aggregate totals like
  `totalCount` via `--json`. If you need a true total, use `gh api graphql`
  to query `totalCount`; otherwise, treat `-L` as the cap for the current call.
- For raw API calls use `gh api --paginate <path>`. Combine with
  `--jq` and (optionally) `--slurp` to assemble one array.

## Repo targeting

`gh` infers the repo from the cwd's git remotes. 

Pass `--repo OWNER/REPO` (`-R`) to override the resolved CWD repo.

## Search vs list

- `gh search issues|prs|code|repos|commits|users` uses GitHub's search
  index and accepts the full search syntax (`is:open`, `author:`,
  `label:`, `repo:owner/name`, `in:title`, ...). Pass each qualifier as
  its own bare token, not as one quoted string:
  `gh search issues repo:cli/cli is:open author:monalisa` works, but
  `gh search issues "repo:cli/cli is:open"` is treated as a single keyword (parsed as `repo:"cli/cli is:open"`)
  and fails with `Invalid search query`. Quote only multi-word free text
  (`gh search issues "broken feature"`). Most qualifiers also have a
  dedicated flag (`--repo`, `--author`, `--label`, ...). Prefer search for
  anything cross-repo or filtered by author/label.
- `gh issue list --search "..."` and `gh pr list --search "..."` take the
  query as one quoted string (it is a flag value) and are scoped to one repo.
- Bots author as GitHub Apps, so `--author dependabot` matches nothing. Use
  `--app dependabot` (on `pr`/`issue list` and `search prs|issues`; expands
  to `author:app/<slug>`) or `--author "dependabot[bot]"`.

## Issue types, sub-issues, and relationships

Newer `gh issue` subcommands model issue types, sub-issue hierarchy, and
blocked-by/blocking relationships.

- `gh issue create`: `--type <name>`, `--parent <number|url>` (creates the
  new issue as a sub-issue), `--blocked-by <number|url,...>`, `--blocking <number|url,...>`.
- `gh issue edit` (edits one or more issues in the same repo, e.g.
  `gh issue edit 23 34`): `--type <name>` / `--remove-type`,
  `--parent <n|url>` / `--remove-parent`,
  `--add-sub-issue <n,n>` / `--remove-sub-issue <n,n>`,
  `--add-blocked-by <n,n>` / `--remove-blocked-by <n,n>`,
  `--add-blocking <n,n>` / `--remove-blocking <n,n>`. Relationship and parent
  refs are issue numbers or URLs; a URL may point to another repo on the same
  host, but a different host is rejected. `--add-sub-issue` cannot be used
  when editing more than one issue.
- `gh issue list --type <name>` filters by issue type.
- `gh issue view` and `gh issue list` accept these as `--json` fields (prefer
  them over scraping the default text output): `issueType`, `parent`,
  `subIssues`, `subIssuesSummary`, `blockedBy`, `blocking`. `subIssues`,
  `blockedBy`, and `blocking` are objects shaped
  `{"nodes": [...], "totalCount": N}` (not flat arrays), and `nodes` is capped
  (`subIssues` at 100, `blockedBy`/`blocking` at 50), so compare the node count
  against `totalCount` to detect truncation.
- GHES: issue types and sub-issues need 3.17+; blocked-by/blocking
  relationships need 3.19+.

## Discussions (`gh discussion`)

Preview command set, subject to change. Subcommands:

- `gh discussion list [--state open|closed|all] [--category <name>] [--author <handle>] [--label <name>,...] [--answered] [--search <query>] [--sort created|updated] [--order asc|desc] [--limit N] [--after <cursor>] [--json <fields>] [--web]`
  lists a repo's discussions. `--state` defaults to open, `--sort` to updated,
  `--order` to desc. `--answered` is tri-state (`--answered=false` for
  unanswered) for Q&A categories.
- `gh discussion view {<number>|<url>|<comment-id>|<comment-url>} [--comments] [--order oldest|newest] [--limit N] [--after <cursor>] [--json <fields>] [--web]`
  shows a discussion's body; add `--comments` for its comments, or pass a
  comment ID/URL as the argument to list that comment's replies (no
  `--replies` flag; `--comments` is rejected with a comment argument).
  `--order` (default newest), `--limit`, and `--after` apply only to comment
  and reply listings.
- `gh discussion create [--title <t>] [--body <b> | --body-file <path>] [--category <name>] [--label <name>,...]`
  creates a discussion. `--title`, a body (`--body` or `--body-file`), and
  `--category` are required non-interactively; omitting any will prompt on a
  terminal.
- `gh discussion edit {<number>|<url>} [--title <t>] [--body <b>] [--body-file <path>] [--category <name>] [--add-label <name>,...] [--remove-label <name>,...]`
  edits title, body, category, or labels.
- `gh discussion comment {<number>|<discussion-url>|<comment-id>|<comment-url>} [--body <b>] [--body-file <path>] [--edit] [--delete] [--yes]`
  adds a top-level comment (when given a discussion) or a reply (when given a
  comment); `--edit` or `--delete` updates or removes a comment/reply and
  needs a comment ID or URL. `--yes` skips the `--delete` confirmation.
- `--json`/`--jq`/`--template` are available on `list` and `view` only;
  `create` and `edit` print the discussion URL. `comment` prints the discussion comment (or reply) URL.

## Reading files and directories (`gh repo read-file` / `read-dir`)

Preview commands, subject to change. They read a repo's contents over the API
without cloning, and honor `--repo OWNER/REPO` (`-R`) and `--ref <branch|tag|commit>`
(default branch when omitted).

- `gh repo read-file <path> [--ref <ref>] [--output <path> [--clobber]] [--allow-escape-sequences] [--json <fields>] [--jq <expr>]`
  prints a file's contents. In non-TTY contexts the raw bytes go straight to
  stdout (pipe-friendly); binary files are written as-is when piped but are
  refused on a TTY. By default, a file containing terminal escape sequences is
  refused; pass `--allow-escape-sequences` to read it anyway. `--output <path>` (`-o`) writes to
  disk instead of stdout (a trailing slash writes under a directory using the
  remote file name; `--clobber` allows overwrite); writing to disk always
  includes the raw bytes regardless of escape sequences. `--output` and `--json` are
  mutually exclusive. `--json` fields include `name`, `path`, `gitSHA`, `size`,
  `type`, `encoding`, and `content` (base64 encoded).
- `gh repo read-dir [<path>] [--ref <ref>] [--json <fields>] [--jq <expr>]`
  lists a directory; with no path it lists the repo root. Non-TTY output is tab
  separated as type, name, octal mode, and byte size. `--json` fields include
  `name`, `path`, `type`, `gitType`, `mode`, `modeOctal`, `gitSHA`, `size`, and
  `submodule`. A path pointing at a file errors and points you at `read-file`
  (and vice versa).

## Fall back to `gh api` for anything `--json` doesn't expose

Sometimes useful data isn't on the typed commands. Examples:

- Review-thread comments on a PR: `gh api repos/{owner}/{repo}/pulls/{n}/comments`
  (the `--comments` flag on `gh pr view` shows issue-level comments only).
- Arbitrary GraphQL: `gh api graphql -f query='...' -F var=value`.
- REST shortcuts: `gh api repos/{owner}/{repo}/...` - note the
  `{owner}/{repo}` placeholder is filled in for you when run from a repo
  with detected remotes; pass them literally if you want determinism.

## Authentication

- `gh auth status` prints the active host(s), user, and which env var (if
  any) is being honored.
- `gh auth status --json` is supported.

## Other notes

- `gh pr checkout <n>` switches branches. Use `gh pr diff <n>` or
  `gh pr view <n>` if you only need to read.
- `NO_COLOR`, `CLICOLOR_FORCE`, and `GH_FORCE_TTY` are honored. Set
  `GH_FORCE_TTY=1` if you want TTY-style output (colors, tables, the
  pager, interactivity) inside an agent harness; leave it unset unless needed.
