# Prototype workflow reference

This file travels with `SKILL.md`. It uses standard command-line tools and
conversation, not Copilot-specific tools, canvases, agents, or other skills.
Use your agent's question/approval interface or ordinary chat. If it cannot
run a command, give the exact command to the user and wait for its result.

## Inspect without changing setup

Set `GH` below to the user's chosen **absolute, durable** prototype path.
These are POSIX-shell examples; do not execute placeholders or use `eval`.
Changing directories does not change which binary is selected.

```sh
GH=/opt/gh-prototype/gh
"$GH" version
"$GH" auth identity create --help
"$GH" auth use --help
git --version
```

Do not assume an installed release contains the feature. A binary in a temporary
directory is unsuitable for persistent bindings: Git will depend on that path.
Help must expose the expected `identity`, `use --view`, `--explain`, and deletion
commands. If it disagrees with this reference, stop and report the mismatch.

Inside the chosen existing project, inspect:

```sh
git -C /home/alex/projects/widget rev-parse --show-toplevel
git -C /home/alex/projects/widget rev-parse --absolute-git-dir
git -C /home/alex/projects/widget rev-parse --path-format=absolute --git-common-dir
git -C /home/alex/projects/widget rev-parse --is-bare-repository
```

Outside a repository, “not a git repository” is expected, not permission to
initialize one. Check whether `.gitmodules` declares submodules. A linked
worktree has its own Git directory under the main repository's common `.git`;
that is supported and identifies the same repository family. Submodules, bare
main repositories, and separate Git-directory layouts remain unsupported.
Inspect the common directory rather than refusing every checkout with a `.git`
file. Prefer a single repository family for the first trial.

Inspect only relevant identity metadata and choices:

```sh
"$GH" auth identity list
"$GH" auth use --list
"$GH" auth use --view --hostname github.com --directory /home/alex/projects/widget
```

These reads do not authenticate. They may warn that a login is missing.
Do not print tokens, `hosts.yml`, full Git config, private keys, environment
values, or arbitrary SSH command strings. Check token-variable **presence only**:
`GH_TOKEN`/`GITHUB_TOKEN` for github.com or `*.ghe.com`, enterprise equivalents
for other hosts. Ask the user to resolve conflicts; do not unset them silently.

`auth status --no-refresh --hostname HOST --json hosts --jq
'.hosts[][] | {login,active,state}'` checks saved accounts **over the network**;
request permission first. `--no-refresh` prevents refreshing credentials, not
network access. If no account is saved, let the user authorize ordinary login
as a separate task. Never retrieve a token to discover the login.

## Supported and deferred

- Inside any checkout/worktree, `auth use IDENTITY` applies to its main checkout
  and all existing/future linked worktrees, anywhere. Reapplying from a sibling
  updates the same choice. There is no per-worktree identity or flag, and no
  need to enable `extensions.worktreeConfig` or change each worktree's config.
- Outside a repository, directory choices are recursive. In a repository they
  match the MAIN/common Git directory location, not the visible worktree path.
  An external worktree inherits its main repository's tree; a worktree placed
  in another bound tree does not inherit that other tree's choice.
- Repository choices win over directory choices (most-specific first), then
  owner fallbacks. A repository choice excludes unrelated nested repositories.
  `--directory` is a target selector: inside a checkout it selects the family.
- Refused: Windows persistence, submodules and repositories
  using them, bare repositories, separate Git directories, unsafe path patterns,
  and incompatible Git/SSH settings. Broad bindings can cause later operations
  in unsupported descendant checkouts to fail closed.
- Existing active non-gh HTTPS helpers are not automatically replaced. Existing
  empty resets to gh are respected. Do not remove GCM/osxkeychain or automatically
  run `auth setup-git` to make a refusal disappear.
- Global compatible SSH commands can be delegated to. Repository-local commands
  and `GIT_SSH_COMMAND` bypass persistent routing and are refused at setup.
  A named wrapper needs a genuine OpenSSH-argument contract; never set
  `ssh.variant=ssh` merely to bypass a check.
- Different-host repository/directory/owner overlaps cannot provide host-specific commit
  attribution. Explain the refusal; do not silently move scopes or erase metadata.

## One complete approved setup

Suppose the user approves existing login `alex-work` on `github.com`, identity
`work-widget`, repository `/home/alex/projects/widget` and all its worktrees, HTTPS, no new Git defaults,
and the global integration effects. No identity/scope already has these names:

```sh
GH=/opt/gh-prototype/gh
"$GH" auth identity create work-widget --hostname github.com --account alex-work
"$GH" auth use work-widget --directory /home/alex/projects/widget
"$GH" auth use --view --hostname github.com --directory /home/alex/projects/widget
"$GH" auth use --explain --hostname github.com --directory /home/alex/projects/widget
"$GH" auth use --explain --hostname github.com --directory /home/alex/projects/widget \
  --json selection,authentication,attribution,warnings
```

Creation alone does not bind or sign in. If applying fails after creation, report
the saved but unapplied identity and offer a scoped recovery; do not broaden
permissions, overwrite user files, or declare success. Keep prior choice names
so an approved replacement can be restored rather than merely unset.

For SSH, add only an existing, explicitly selected `--ssh-host ALIAS` to the
approved create/edit command. For attribution, add both explicit
`--git-user-name NAME` and `--git-user-email EMAIL`. Offer an exact address,
`@public` for the identity account's public profile email, or `@noreply` for its
ID-based github.com noreply address. Selectors perform a network request using
that saved account, may refresh its credential, and save the concrete result
once; obtain approval for that lookup as part of the setup plan.
Do not construct the address yourself. Missing public email fails without a
fallback; let the user choose another source. `@noreply` is github.com-only;
other hosts need an explicit address. The Git name remains user-supplied.
Changing account/profile does not automatically refresh the saved email.
Do not infer author/committer
separation: these are shared Git defaults, subject to Git/environment overrides.

## Verify exactly what was requested

Always supply `--hostname` for inspection/unset: its default is github.com, not
the remote or `GH_HOST`. Directory inspection uses `--directory`; owner-context
inspection uses `--owner` from the relevant current directory, and directory
precedence still applies. Do not combine those scope flags.

Verify selected identity, matching scope, account, SSH route, attribution and
warnings. Outside a repository, Git name/email are the identity's defaults,
not evidence that Git applied them. JSON status labels are not authentication
proof. Explain failures and missing account recovery without deleting preferences.
For repository families, inspect main and at least one existing linked checkout
when available; `context.commonGitDir` must agree, and
`selection.selected.repository` identifies a repository-specific choice.
`context.scopeDirectory` shows where directory inheritance is matched.
Ask before creating a new worktree solely for verification.

Optional, separately approved live verification:
- API: `"$GH" api --hostname HOST user --jq .login` from the chosen project.
  It may refresh the selected credential in its configured storage.
- Read-only Git access: `git ls-remote origin` for an explicitly agreed remote.
  This tests access, not which account an SSH key belongs to.
- SSH account verification needs a separate explicit probe against the selected
  alias. Warn that SSH config can run `Match exec` and even inspection with
  `ssh -G` can execute local commands; don't launch it automatically.

Do not run `identity-e2e`, native acceptance, `make debug`, or browser logins as
normal onboarding. Those are developer validation workflows, not setup requirements.

## Commands and scope of undo

| Intent | Command using the chosen `GH` |
| --- | --- |
| Remove a repository-family choice from any checkout, or a directory-tree choice outside a repo | `auth use --unset --hostname HOST --directory PATH` |
| Stop using one owner choice | `auth use --unset --hostname HOST --owner OWNER` |
| Remove an identity and every binding using it | `auth identity delete NAME` |
| Clear all identities/bindings across this gh configuration | `auth identity delete --all` |
| Inspect why a choice applies | `auth use --explain --hostname HOST` |

Read current bindings before undo; other work may have reused the identity.
Unsetting from a worktree affects its entire repository family, not just that
checkout. If asked to undo only one worktree, explain that narrower scope is
not supported and request approval before changing siblings.
Named deletion requires retyping the name; bulk deletion requires `all identities`.
Without a terminal, `--confirm` is required, but ask for approval of the exact target
before supplying it. Never use bulk deletion to undo one assisted setup.
Last-binding removal detaches gh's global integration; it does not delete
credentials or keys. Logout keeps identities, and may leave SSH access working.

## Sharing the draft skill

Copy the **whole** `setting-up-identites` directory into the chosen agent's
supported skill directory, keeping `references/workflow.md`. Follow that agent's
installation instructions; there is no universal slash-command syntax.
For an agent without skills, attach both Markdown files as user-requested
guidance. This does not install gh or make released versions support identities.
This checkout is uncommitted: use a local copy/attachment, not a fabricated
GitHub download link. Review the skill before granting it access to your setup.
