---
name: setting-up-identites
description: Use when a user wants guided setup of GitHub CLI prototype identities, saved accounts, repository/worktree, directory or owner bindings, SSH aliases, or Git name/email preferences, or wants to inspect or undo that setup.
---

# Setting up GitHub CLI identities

Guide setup of this **unreleased prototype**, not released `gh`. Use plain
language; ask one focused question, then wait. Read the bundled
[workflow reference](references/workflow.md) before proposing commands.

## 1. Establish the target

Establish the executable and project. Use the same absolute path throughout. Check
`version`, `auth identity create --help`, and `auth use --help`; inspect Git's
version and repository layout without writes.

Require Git 2.36+, macOS/Linux, and repositories without submodules.
Linked worktrees are supported: choices inside any checkout affect the entire
repository family, including external and future worktrees.
Stop on unsupported layouts or incompatible help.
Do not install/upgrade software, replace their installed `gh`, or run the live
test harness as onboarding.

## 2. Choose the identity

Establish host, saved login, identity name, repository/directory/owner scope, optional SSH
alias, and optional Git name/email. Ask only for missing decisions. Inspect
existing identities/choices first. Reuse suitable identities; shared edits
affect every place using them.

Never guess email, alias, or account. Offer a literal Git email, `@public`, or
`@noreply` (github.com); selectors require approval for the network lookup.
Keep the Git name explicit. Omit defaults when existing attribution is wanted.
Broader settings can still apply. An alias
doesn't prove which account its key authenticates as.

## 3. Obtain specific approval

Before **any configuration mutation**, present:

| Decision | Required answer |
| --- | --- |
| Identity | Create, reuse, or edit; name, host, login |
| Scope | Repository/all worktrees, directory tree, or owner; existing choice replaced |
| Git defaults | Exact name and literal email/source, or unchanged; lookup/inheritance noted |
| SSH | Alias or HTTPS-only; existing setup preserved or unsupported |
| Global effects | Git include, host-wide HTTPS helper, all-directory SSH dispatcher |
| Undo | Exact scope to unset, including all affected worktrees; whether identity is shared |

Obtain approval of the commands and effects. **HTTPS-only still installs the
SSH dispatcher.** “Don't change SSH” conflicts with persistence: explain and stop.

Decline shortcuts that remove credential helpers, unset user environment
variables, rewrite Git/SSH files, or delete other identities to bypass errors.
No token/private-key output, credential-file dumps, login/logout, account
switching, key generation/uploads, or remote mutations as implicit setup.

## 4. Apply and verify

Run approved commands individually from the agreed context. Shell-quote user
values; check exit status. Stop on failure and report what changed.

Inspect `auth use --view`, `--explain`, and explanation JSON with explicit host
and scope. Compare with the plan. Configuration evidence isn't live
authentication. Ask separately before API/SSH probes; don't verify by creating
repositories, pushing, or making PRs.

Finish with identity/scope, observations, unverified assumptions, and precise
undo. Don't automatically clean up successful setup.

## Common mistakes

- Bare `gh` after changing directory can invoke the wrong binary.
- `--account` is authentication for one command, not a persistent binding.
- `identity delete NAME` also deletes **all** its bindings; scoped undo uses
  `auth use --unset`, not deletion.
- Never pass `--confirm` or confirm typed deletion without approval of that exact
  deletion scope. `--all` affects all hosts in the current gh configuration.
