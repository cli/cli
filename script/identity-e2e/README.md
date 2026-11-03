# Reusable identities: local UX prototype

This is an experimental multi-account prototype for review, not a release-ready
implementation. **Use a disposable
environment. `gh auth use` writes persistent Git configuration.** The scenarios
below isolate those writes from your normal configuration.

## Tester handoff

For agent-guided onboarding, use the portable
[`setting-up-identites` skill](../../.github/skills/setting-up-identites/SKILL.md).
Copy its entire directory (including `references/workflow.md`) to your chosen
agent's supported skill location, following that agent's instructions. If it
does not support skills, attach those two Markdown files as guidance. This
draft skill does not install the prototype, run the live test harness, or change
configuration without approval. No universal slash command or published
download is assumed.

For ordinary prototype use, build with `make` and use the resulting executable
without replacing your installed release of gh:

```sh
./bin/gh auth identity create demo --account YOUR_SAVED_LOGIN
./bin/gh auth use demo --directory /PATH/TO/STANDALONE/PROJECT
./bin/gh auth use --explain --directory /PATH/TO/STANDALONE/PROJECT
./bin/gh auth identity delete demo
```

For an SSH project, configure its SSH alias first and add `--ssh-host ALIAS`
when creating the identity. The alias must select the intended key/account;
the prototype cannot verify that declaration. Existing accounts may use normal
credential refresh. Identity setup/deletion itself does not sign accounts in or out.

**Read before applying:** setup adds a global Git include, a host-wide gh HTTPS
helper, and an SSH dispatcher used in all directories. Existing non-gh helpers
for the host are refused, not replaced. Keep the executable at its current path
while bindings exist. The last binding removal detaches the managed integration;
`auth identity delete --all` removes all identity choices in this gh config.
Deletion uses typed confirmation or explicit `--confirm`.

Supported evaluation: Git 2.36+ on macOS/Linux, repositories and linked worktrees, recursive directory
choices, owner fallbacks, identity edits/deletion, explanation JSON, API/HTTPS
account overrides, and SSH via OpenSSH or explicitly compatible wrappers.
Submodules, repositories using submodules, separate Git
directories, bare-repository identities, Windows persistence,
custom non-gh credential-helper chains, and overlapping different-host directory
attribution are refused. A broad tree containing unsupported checkouts will
also refuse identity-based operations inside them; use a standalone project
directory for the first trial. Arbitrary SSH shell programs and CA/key verification,
cross-process configuration transactions, and production rollout are deferred.

An earlier prototype's generated includes are not automatically migrated.
Before upgrading an existing experiment, remove its identities with
`auth identity delete --all`, then recreate the intended choices with this build.
Do not manually copy old generated configuration into a fresh evaluation.

The live journeys below are developer validation tools, not a prerequisite for
using the prototype. They require a debug build that can isolate OS keyring access.

## Guided live walkthroughs

These adapt the account setup and guided review from the previous
`multiaccount-e2e` runner: healthy-account discovery and reuse, distinct
browser-login fallback, redacted token capture, isolated account restoration,
and guided exploration of identity behavior.

```sh
make debug
script/identity-e2e/run.sh --list
script/identity-e2e/run.sh --scenario https-directory
script/identity-e2e/run.sh --scenario https-owner
script/identity-e2e/run.sh --scenario ssh-directory
script/identity-e2e/run.sh --scenario ssh-owner
script/identity-e2e/run.sh --scenario https-worktrees
script/identity-e2e/run.sh --scenario ssh-worktrees
script/identity-e2e/run.sh --scenario https-account-override
script/identity-e2e/run.sh --scenario ssh-account-override
script/identity-e2e/run.sh --scenario ssh-cooperation
script/identity-e2e/run.sh --scenario logout-recovery
script/identity-e2e/run.sh --scenario identity-lifecycle
script/identity-e2e/run.sh --all --keep
```

Requires Bash 4.4+ (use Homebrew's Bash on macOS, not `/bin/bash`), Git, jq,
and this worktree's debug-built `bin/gh`. SSH journeys additionally require OpenSSH
and `ssh-keygen`. `--gh PATH` selects a different executable.

**The shell runner uses real GitHub accounts and resources.** Running it is
permission to create its temporary private repositories/PRs and, for SSH
journeys, upload temporary public keys. It has not been run against the
operator's credentials by the agent.

**The runner refuses binaries without keyring-isolation support.** It verifies
the capability before account setup, then disables OS keyring reads, writes, and
deletes for every isolated bootstrap/scenario command. This is a debug-build-only
facility (`GH_INTERNAL_DISABLE_KEYRING=true`), not a new meaning of
`--insecure-storage`. Normal saved-account discovery and token capture explicitly
disable refresh; the selected tokens are then restored into private scenario files.
Without this runner/debug facility, `HOME`/`GH_CONFIG_DIR` alone do not isolate
ordinary login/logout from the OS keyring.

Before isolating configuration, the runner lists healthy saved `github.com`
accounts and offers to reuse exactly two. With more than two it asks which
two. Invalid accounts are excluded. Declining, having fewer than two healthy
accounts, or an unavailable captured token triggers two browser logins in
isolated bootstrap configuration. It checks that the two logins really differ
before capturing tokens, redacts the token output, and logs out both bootstrap
accounts. Every scenario then restores those two tokens using
`auth login --with-token --insecure-storage`, without another browser login.

The Git journeys use the original preflight's `repo`, `delete_repo`, and
`admin:public_key` scope requirements. `identity-lifecycle` and `logout-recovery`
are API-only and require no extra scopes. One compatibility exception to the copied setup:
the other prototype's named-account refresh command does not exist here.
Selecting its scope-refresh offer explains this and takes the existing
fresh-browser-login fallback rather than switching or refreshing an operator
account.

Account setup and SSH preparation run as ordinary setup, without Given / When /
Then blocks or extra Enter prompts. Account-selection and browser-login prompts
remain. SSH preparation still fetches trusted host keys and checks both
accounts' aliases, without dumping host keys or resolved SSH configuration.

Each identity-behavior step shows Given / When / Then and its exact commands,
pauses before execution, runs the commands with their output and PASS checks,
then pauses again. Enter advances; `e` opens the same exploration prompt as before:

| Input | Effect |
| --- | --- |
| Enter | Return to the guided step. |
| A shell command | Run it from the displayed scenario directory. |
| `:state` | Inspect accounts, identities, selection explanation, bindings, Git config, and repository state. |
| `:shell` | Open an interactive shell with the isolated environment; exit to return. |
| `:quit` | Stop and perform cleanup. |

`--non-interactive` skips the journey pauses, but does not silently select
operator accounts: the original account-reuse choice still requires a terminal.
The account-override and SSH-cooperation journeys require a terminal and also ask where to push inside `gh pr create`;
select the displayed fixture repository. They intentionally do not pre-push the
feature branch or use `--head`, which would bypass the behavior under test.
Interactive lifecycle and cooperation cleanup asks for gh's own typed
confirmation: the identity name, or `all identities`. With `--non-interactive`,
those deletion commands explicitly pass `--confirm`; this does not bypass account
selection or PR push-selection prompts.
Run `--help` for all options.

| Live scenario | What to look for |
| --- | --- |
| `https-directory` | Outside a repository, bare `gh auth use IDENTITY` selects a directory tree. Unqualified `gh repo create NAME --private` infers the selected account as owner; gh clones it; ordinary Git commits/pushes; gh creates a PR with the matching author. |
| `https-owner` | Owner bindings select account and commit metadata for private repositories without a directory binding. |
| `ssh-directory` | The directory journey with isolated keys/SSH aliases, verified against GitHub before applying identities. |
| `ssh-owner` | Owner-selected SSH routing; canonical remotes stay unchanged while independent Git uses the account's alias. |
| `https-worktrees` / `ssh-worktrees` | Existing/future worktrees outside the main repository's tree inherit its directory identity. Applying from any sibling selects one account/attribution profile across the family; unset/delete reveals the directory fallback. Live API and independent Git access are checked. |
| `https-account-override` | API override without identities, then override a directory identity to clone another account's private repository. Let `gh pr create` push an unpublished branch and create the PR as that account; attribution and persistent selection remain unchanged. |
| `ssh-account-override` | The same override journey using the requested account's uniquely declared SSH alias. Independently invoked Git afterward still uses the persistent identity and cannot access the other account's private repository. |
| `ssh-cooperation` | The SSH override journey through an explicitly OpenSSH-compatible connection wrapper. Both gh's automatic PR push and independent Git invoke it. Deleting all identities restores the original connection command. This does not verify an enterprise CA policy. |
| `logout-recovery` | Logout retains identities and bindings, reports recovery options, and does not change attribution. Missing-account API requests fail rather than using the other saved account. Signing back in repairs the choice without rebinding; deleting it keeps credentials. |
| `identity-lifecycle` | No GitHub repositories or keys: nested selection, plain and JSON explanations, inherited attribution, shared-account profiles, live edits, unsetting, typed named deletion revealing a broader choice, and typed full cleanup retaining saved accounts. |

Both directory journeys create with only a repository name, not `OWNER/NAME`,
and check the returned URL before cloning. The current directory is not a
repository at that point. The authenticating account selected by the directory
choice becomes the default owner; an explicitly supplied organization owner is
still independent of the authenticating account.

Owner-only setup deliberately creates its private fixture repositories using
an explicit maintenance account; it does not pretend a context-free
`repo create` query is owner-selected. Clone, in-repository API reads, ordinary
Git pushes, and PR authorship exercise implicit identity selection.

Each invocation creates a unique child beneath `--run-root` (or the temporary
directory). It never reuses/deletes an existing parent's contents. Each
scenario gets its own HOME, gh config, and global Git config. The runner also
enables the debug-only keyring isolation described above. SSH configuration and private keys
live inside the scenario; a PATH-local `ssh` wrapper supplies `-F` to the real
OpenSSH executable, including when called by the identity dispatcher. This is
necessary on macOS, where overriding HOME does not redirect OpenSSH's default
config lookup. No `GIT_SSH_COMMAND` bypass of the dispatcher is used.

SSH host keys come from GitHub's HTTPS `meta` endpoint rather than an
unauthenticated `ssh-keyscan`. Each generated alias is checked with `ssh -G`
and `ssh -T`; GitHub must greet the expected login. The upload is registered
before making the request so interruption can recover its uniquely titled key.

Cleanup deletes only this run's registered repositories and uploaded keys,
then its own local directory. `--keep` preserves **both live resources and
plaintext credentials/private keys**. Failed cleanup preserves local state
and lists exact remaining resources in `cleanup-needed.txt` rather than
discarding the credentials needed for recovery.

Exploration and `:shell` put a run-local `gh` launcher on PATH, so typing `gh`
exercises the selected `--gh` binary rather than an installed release.
Git repository discovery stops at the run directory: even if `--run-root` is
inside an existing checkout, the outside-repository scenarios cannot inherit
that checkout's identity or repository context.

## Credential-free regression journeys

Go tests provide offline regression journeys, separate from the guided live
walkthrough. Build the prerelease first and pass its absolute path:

```sh
make
go test ./script/identity-e2e -run '^TestOffline' -count=1 -args -gh "$PWD/bin/gh"
go test ./script/identity-e2e -run '^TestOfflineScopes$' -v -count=1 -args -gh "$PWD/bin/gh" -keep
go test ./script/identity-e2e -run '^TestRunner' -count=1
```

`offline_test.go` uses fake accounts, simulated HTTPS/SSH transports, and no GitHub
resources. It runs real `git credential fill`, commits, and clones of empty
simulated repositories, not libcurl or real key authentication. Without `-gh`,
these journeys skip rather than using an installed executable. They require
macOS or Linux.

With `-keep` and verbose test output, each journey prints a fixture `enter.sh` to source in a disposable
subshell for offline exploration. HOME, gh config, Git helpers, and SSH are
redirected into the fixture; `.invalid` hosts and tokens are fake. Do not
replace those hosts with live ones. Each case is independent; use Go's `-run`
regular expression to select journeys.

`runner_test.go` exercises the **real adapted bootstrap and runner cleanup** in a
pseudo-terminal with a fake gh executable, never operator credentials. It
checks accepted/declined reuse, unhealthy/missing/unavailable tokens, duplicate
browser accounts, selection among more than two accounts, guided exploration,
correct exploration-binary selection, enclosing-repository isolation, scope
fallback, unique run-directory cleanup, key-upload recovery, and recoverable
resource deletion failures. Interactive account and SSH setup run without
scenario-review pauses, using fake SSH commands for the SSH checks.
The runner tests also ensure interactive deletion leaves typed confirmation to
gh, while unattended deletion explicitly passes `--confirm`.

| Case | What to look for |
| --- | --- |
| `repository` | Apply from a subdirectory: the choice covers this repository family, not unrelated nested repositories. Two identities share one account but use different Git metadata and SSH routes; saved URLs stay canonical. |
| `scopes` | Owner default, broad directory choice, repository choice, and removing higher-precedence choices to reveal fallbacks. Git metadata and SSH routing follow the same precedence. |
| `owners` | Owner binding activates for HTTPS, scp-style SSH, `ssh://`, and organization-specific SSH certificate users; independent Git obtains the selected account's credential. |
| `live-edits` | Editing account, email, and SSH host updates two existing bindings; confirmed deletion removes the identity and both bindings. |
| `account-only` | Switching to an account-only identity removes the previous identity's metadata and reveals ordinary Git defaults. |
| `nested-trees` | More specific directories win regardless of creation order. Explanations from the child show ancestor precedence; explanations from the parent show descendant exceptions. Unsetting the child reveals the parent for both gh and Git. |
| `ambiguous-owners` | A repository with differently bound remote owners needs an explicit repository identity for context-free selection. |
| `failures` | Bare use outside a repository, plus rejection of unknown saved accounts, missing SSH routes, conflicting credential usernames, environment-token overrides, conflicting scope flags, and unsafe identity names. |
| `git-clone` | Independently invoked Git clones over both transports; caller/owner scope authenticates clone, destination scope applies to later operations. |
| `inherited-metadata` | Explicitly demonstrates the unresolved case where a narrower account-only identity inherits a broader identity's commit metadata. |
| `account-override` | Real gh-launched Git uses the requested account over HTTPS without identities, then SSH with declared routes. Subsequent independent Git, selected identity, attribution, remote URL, and saved accounts are unchanged. The empty remote intentionally rejects a missing branch after transport authentication. |
| `cleanup` | Named deletion preserves other choices; `delete --all --confirm` clears identities and bindings, restores ordinary Git behavior, and is safe to repeat. |
| `repository-family` | External worktree directory inheritance, shared main/existing/future selection, native Git commits, HTTPS credentials, SSH aliases, scope JSON, replacement/unset/delete, and unchanged repository Git configuration. |

The Go root-command regression `TestAccountOverridePRCreatePushesBeforeCreatingPR`
also executes the real `pr create` command from an external linked worktree,
with real Git pushes into local bare
repositories over simulated HTTPS and SSH. Mock API responses and fake
credentials keep it offline; the PR mutation checks that the feature ref was
actually pushed first.

## Recent decision coverage

| Decision | Scenario / acceptance coverage | Focused checks retained |
| --- | --- | --- |
| Explicit `--view`, `--explain`, JSON fields and `--jq` | `identity-lifecycle`; native `identity/identity-lifecycle.txtar` | TTY colours vs plain text, flag conflicts, graph origins and manual include drift |
| Authentication-only `--account`, including gh's automatic PR push | `https-account-override`, `ssh-account-override` | Real Git into local bare repositories with simulated HTTPS/SSH; unchanged commits and configuration |
| One repository choice across main/existing/future worktrees, regardless of checkout location | `https-worktrees`, `ssh-worktrees`, offline `repository-family`, native `identity-lifecycle.txtar` | Main/common-directory precedence, API transport, sibling configuration preflight, unrelated nested repository isolation, replacement/unset/delete |
| Contextual/unique SSH routes; no global `--ssh-host` escape hatch | Offline `account-override` | Distinct per-target routes, shared aliases, missing/ambiguous/conflicting declarations |
| Preserve compatible SSH commands and their options | `ssh-cooperation`; offline `account-override` | Underlying command edits, local/environment overrides, literal paths, unsupported variants, recursion, include-order rollback |
| Logout keeps preferences; inspection diagnoses missing accounts | `logout-recovery`; native `identity/identity-lifecycle.txtar` | Real logout command with isolated config/fake keyring; retained binding and SSH/attribution distinction |
| Named deletion cascades only its bindings; full cleanup removes debris | `identity-lifecycle`; offline `live-edits` and `cleanup`; native `identity/identity-lifecycle.txtar` | Typed name/all confirmation, cancellation, required `--confirm`, locked-config rollback, unexpected-settings preservation |

The normal live acceptance script is
`acceptance/testdata/identity/identity-lifecycle.txtar`. It uses the existing
acceptance harness, requires a **user-capable token**, and creates no remote
repositories or SSH keys (`fixture-repo none`). It signs in, checks live API
authentication, applies and inspects identities, logs out, signs back in, and
deletes named/all identities while checking that API authentication survives.
Run it alone with the `identity` group and `identity-lifecycle` script filter,
using `-tags=acceptance,ghdebug`. It refuses to start without the debug keyring
isolation capability and enables it before login/logout. The token goes through
a private temporary stdin file, never an external `echo` argument. It is not runnable with the usual
installation-token acceptance runner, and it skips Windows because the
prototype is not qualified there.

The new live acceptance script and new guided journeys have **not been run
against live GitHub**. Acceptance metadata, shell validation, runner tests, and
existing local behavioral tests are separate evidence, not proof of live
authentication. API gateway and gist credential ownership have dedicated
credential-free regressions; live tests do not establish arbitrary enterprise
gateway, wrapper, or key compliance.

## Candidate interface

```sh
gh auth identity create work \
  --account alice \
  --hostname github.com \
  --ssh-host github-work \
  --git-user-name "Alice Example" \
  --git-user-email alice@work.example

gh auth use work                      # In a checkout: main repo + all worktrees; outside: directory tree
gh auth use work --owner company      # Owner on the identity's GitHub host
gh auth use work --directory ~/src/repo

gh auth identity edit work --git-user-email new@work.example
gh auth identity edit work --git-user-email @public
gh auth identity edit work --git-user-email @noreply
gh auth identity edit work --ssh-host github-work-new
gh auth identity edit work --clear-git
gh auth identity view work
gh auth identity list

gh auth use --view                    # Current selection, as JSON (also the default with no arguments)
gh auth use --explain                  # Persistent selection, routing, attribution, and Git includes
gh auth use --explain --directory ~/src/repo
gh auth use --list                     # All bindings, as JSON
gh auth use --unset                    # Remove this family/directory choice; reveal fallbacks
gh auth use --unset --owner company
gh auth identity delete work          # Retype "work" to delete it and its bindings
gh auth identity delete --all         # Retype "all identities" to remove all identity configuration
gh auth identity delete --all --confirm # Explicit confirmation for scripts

gh api --account bob user --jq .login  # Saved account; no identity required
gh pr create --account bob            # Includes gh's Git push, not commit attribution
```

Git name/email must be supplied together when creating an identity; both are
optional. Editing one retains the other. Multiple identities may reference the
same saved account. `--account` chooses the saved GitHub account for
authentication. `--git-user-name` and `--git-user-email` set Git's `user.name`
and `user.email` attribution defaults, including commits, annotated tags, and
reflog records; they do not choose an authentication login.
An SSH route is optional for HTTPS-only use, but attempting
SSH with a selected identity lacking a route fails instead of trying an
unrelated default key.

### Choosing an email from the account

`--git-user-email` accepts a literal address or an explicit lookup selector:

| Value | Result |
| --- | --- |
| `alice@example.com` | Stored as supplied, without a profile lookup |
| `@public` | The account's public profile email on the identity's host |
| `@noreply` | `ID+LOGIN@users.noreply.github.com`, constructed from the account profile on github.com |

Selectors are supported on create and edit. They use the identity's saved
account/host, even if another account is active or an environment token is set.
They perform a network lookup, which may refresh that saved credential. They
never fetch the list of private email addresses. A Git name must be supplied or
already stored; the profile's name does not replace it.

The resolved address is printed and stored as a concrete value, not a live
reference. Changing a GitHub profile or editing only the identity's account
does not silently change the email. Pass the selector again to resolve it again.
Missing public email, unavailable credentials, or failed profile lookup leave
the identity unchanged; the command explains noreply/literal alternatives.

`@noreply` is github.com-only in this prototype. Other hosts must use a literal
address from their email settings. It uses the modern ID-based form, not an
account's legacy username-only noreply preference; see
[GitHub's email address reference](https://docs.github.com/en/account-and-profile/reference/email-addresses-reference#your-noreply-email-address).
It does not change privacy settings or guarantee compliance with an enterprise's
allowed commit-email policy.

The guided `identity-lifecycle` journey exercises both selectors, including
the explicit no-public-email failure when applicable. Mocked command tests
cover both public-email outcomes, chosen-account/host routing, fixed-value
edits, and invalid profile responses without using live credentials.

## Inspecting persistent selection

`gh auth use --explain` renders a shared structured report with separate
sections for context, selection precedence, authentication, attribution, and
configuration sources. TTY output uses bold headings, cyan values, muted
provenance, and yellow blockers/warnings. Plain output retains the same labels
without relying on colour.

The human-readable output shows account names directly rather than internal
status labels such as `[declared]` or `[blocked]`. Missing SSH configuration is
described as optional setup, with the command to configure it. Outside a
repository, the report says "You're not in a Git repository. This identity
sets:" before listing its name/email settings. The include graph marks files
that supplied no settings here; it does not add a generic explanation about
future repositories. The JSON status codes and `targetObserved` field remain
unchanged.
For a directory-based include outside a repository, the graph gives the reason:
"Not applied: this directory is not a Git repository." It does not use this
reason for other conditions or groups containing unconditional includes.

```sh
gh auth use --explain
gh auth use --explain --json context,selection,authentication,attribution,configuration,warnings
gh auth use --explain --json selection --jq '.selection.identity.name'
gh auth use --explain --json configuration
```

The report describes **persistent selection**, not `--account` overrides.
Authentication effects distinguish `declared`, `blocked`, `overridden`, and
`unmanaged` states. It checks whether the referenced account is still registered
locally, but does not check token validity or authenticate an SSH key.
Attribution is `observed` Git defaults inside a repository, `declared`
identity defaults outside one, or `unavailable` without either. Author and
committer environment overrides are listed by variable name, never their values.

Configuration edges come from Git's actual include directives and origins.
`targetObserved` means Git reported settings from that target file during this
inspection, not that this particular include condition matched. An empty file
may therefore be unobserved. Outside a repository, conditional includes may
apply later in descendant checkouts. Repeated targets are grouped in text;
JSON retains all conditions. Removing a managed include does not manufacture
an installed graph from the saved binding: the report warns when the selected
identity file is not observed inside the repository.

`--json <fields>`, `--jq`, and `--template` use the standard gh exporter and
require `--explain`. The previous `--explanation` spelling remains a hidden
compatibility alias. Bare `auth use` and `--list` retain their existing JSON.
`--view` explicitly requests the same selected-identity JSON as bare `auth use`;
it cannot be combined with another action such as `--explain` or `--unset`.

## Removing identities and recovering from missing accounts

### Return to stable gh

Run this with the prerelease **before** replacing or uninstalling it:

```sh
gh auth reset-multi-account-experiment
```

Retype `all identities` to confirm, or pass `--confirm` for a noninteractive reset.
This is the same cleanup as `gh auth identity delete --all`, not another
configuration-removal implementation. It removes identity choices and managed
Git integration but keeps accounts, credentials, and the user's own Git/SSH
settings. It does not uninstall gh.

If you have already installed an older version, restore or run the prerelease
binary to reset first, then return to the older version. Older releases do not
understand the experimental SSH dispatcher. Replacing the binary without
resetting can therefore break ordinary Git SSH operations until cleanup.

The credential-free downgrade experiment is repeatable with an existing old
binary (it never replaces your installed executable):

```sh
go test ./script/identity-e2e -run '^TestResetDowngrade$' -count=1 \
  -args -gh "$PWD/bin/gh" -old-gh /absolute/path/to/old/gh
```

Observed against gh 2.102.0: old binary baseline, prerelease setup, an intentionally
unreset downgrade that fails, prerelease reset, then old binary recovery all behaved
as expected. The test uses copies at one temporary installation path, fake
credentials, simulated HTTPS/SSH transports, and local Git repositories.
It compares original saved accounts/configuration, credential selection, aliases,
SSH command/options, and commit attribution. This is not live authentication or
a guarantee about every package manager/configuration combination.

### Remove individual choices

`gh auth identity delete NAME` removes that identity and every repository, directory or
owner binding that references it. Other identities and bindings remain.
Affected locations fall back to other applicable choices or ordinary gh/Git
settings. The command lists affected bindings and asks you to retype the name.
Use `--confirm` to skip the prompt; it is required without an interactive terminal.

`gh auth identity delete --all` removes every identity and binding across all
hosts in the current gh configuration. Its typed confirmation is `all identities`.
`--all --confirm` is the non-interactive form. There is no implicit deletion target
and no separate `--delete-all` or `--unset-all` flag.

When the last binding is removed (including through `auth use --unset`), gh
removes `auth.gitconfig` and its own global include entry, so no helper or SSH
dispatcher configuration is left behind. Bulk deletion also removes identity
files and the identities directory if empty. Saved accounts, credentials,
SSH keys, SSH configuration, and unrelated Git settings are not removed.
Unknown files in the identities directory are retained.

Cleanup refuses unrecognized ownership, non-regular managed files, malformed
scope metadata, and unexpected settings added to the generated integration
file. Its errors identify what needs manual attention rather than deleting
unrelated configuration. A locked Git configuration causes attempted deletion
to roll back. Multi-file cleanup is not a cross-process transaction or
crash-recovery guarantee; errors after detachment identify any remaining
temporary files.

Logging out keeps identities and bindings. Logout now lists affected identities
with commands to sign back in, change the referenced account, or delete the
identity and its choices. `auth identity list/view` and `auth use --view` warn
on stderr when an account is no longer signed in while retaining their JSON
output. `auth use --explain` includes the problem in both text and JSON, marking
API and HTTPS authentication unavailable. Credential lookup reports recovery
guidance rather than falling back to another account.

SSH keys and Git attribution remain independent: SSH may still work after
logging out of gh. Signing back into the same host/account makes the retained
configuration usable again; inspection does not test token validity.

## Invocation-local account overrides

The global `--account LOGIN` selects a saved account on the command's GitHub
host for API requests and gh-launched Git. It takes precedence over contextual
identities and environment tokens, but does not overwrite explicit HTTP
Authorization headers. Unknown accounts and unavailable credentials fail
instead of falling back. Other hosts retain their own selection.

API and HTTPS overrides work without creating an identity. SSH needs a route
declared by an existing identity for that host/account. Multiple identities
sharing one alias count as one route. When several aliases exist, matching
repository choices for the requested account take precedence over directory
choices, then owner choices; no matching context means an ambiguity error. A route declared
for different accounts or hosts is rejected. Each SSH target is resolved
separately, so a fork and upstream can use different contextual routes.

There is no global `--identity` or `--ssh-host` override. To resolve a missing or
ambiguous route, create/edit an identity's `--ssh-host` declaration and apply
the intended identity with `gh auth use`. An alias is a declaration, not proof
that a key authenticates as that account or satisfies an enterprise policy.

The override leaves active-account configuration, identities, repository/directory/owner
choices, remote URLs, and Git attribution unchanged. A later independent Git
command follows the persistent choice. Ordinary Git effects of the requested
command, such as a push setting branch tracking, still occur.

Routing uses the common Git client, including its copies, rather than a
clone-only wrapper. It preserves literal OpenSSH command arguments from
`GIT_SSH_COMMAND`, `core.sshCommand`, or `GIT_SSH`. Other command names can be
used when `ssh.variant=ssh` (or `GIT_SSH_VARIANT=ssh`) explicitly declares that
they accept OpenSSH arguments and honor the supplied destination. Unsupported
variants, unknown argument conventions, shell expansions, pipelines, and
redirections fail. An SSH
destination must be the selected host or a declared host/alias; unknown
destinations, including undeclared submodule hosts, fail
rather than bypassing account selection. URL rewrites are handled at the
actual SSH invocation.

Authentication-management commands do not accept the global override. Their
existing options still apply, including `auth identity create/edit --account`
and `auth token --user`. This avoids treating an authentication override as a
request to switch, log out, or refresh a different saved account.

Directory choices are always recursive, including future repositories. There
is no `--recursive` flag. `--owner` is independent of the current directory and
mutually exclusive with `--directory`. Inspection and `--unset` default to
`github.com`; supply `--hostname` when using another host. Applying a named
identity always uses that identity's host.

Inside a repository or linked worktree, applying creates one **repository
choice**, keyed by its canonical common Git directory. It covers the main
checkout and all existing/future linked worktrees anywhere. Applying from any
sibling replaces the same choice. `--unset` from any sibling removes it across
the family. There is no worktree-specific identity or flag, and gh does not
enable `extensions.worktreeConfig` or write individual `config.worktree` files.

Outside repositories, applying creates a recursive directory choice. In a
repository, directory inheritance is matched against the main repository/common
Git directory's location, not the visible linked checkout. A worktree placed
under another bound folder keeps its main repository's choice.

Precedence is repository > deepest directory > broader directories > owner.
Repository scope excludes unrelated repositories nested in a checkout.
`--directory` selects a target, not a forced directory scope: targeting an
existing checkout selects its repository family.
Setup inspects existing registered worktrees for incompatible connection
settings before applying a repository choice. It does not overwrite their Git
configurations. Later user changes, including `config.worktree` name/email or
SSH overrides, retain normal Git precedence and are visible in explanations.

Existing stored directory records remain directory records; they are not
silently converted. Reapplying within a repository creates a higher-precedence
family record. Rewriting bindings regenerates the native include patterns for
worktree inheritance. For a clean prototype upgrade, delete old choices and
recreate them; repository moves still require recreating path-based bindings.

### Explain a choice

`--explain` is read-only and uses the same resolution result as API,
credential, and SSH selection. It shows the current path and repository root,
the visible checkout, shared Git directory, directory-inheritance location,
winning repository/directory/owner choice, and lower-precedence matches.
From a parent directory it separately lists descendant exceptions: those do
not change the parent's current selection.

For example, after selecting `work` under `~/work` and `client` under
`~/work/client`, an explanation inside `~/work/client/project` describes:

```text
Using identity "client" (bob on github.com)

Why this identity:
  Selected: directory /Users/alice/work/client (includes descendants)
  Overrides directory /Users/alice/work -> work: more specific directory
```

The consequences include the configured API/HTTPS account, declared SSH route,
and Git's actual effective `user.name`/`user.email` with their configuration
origins. Thus inherited metadata and repository-local overrides are visible,
not inferred from the winning identity file. Outside a repository, it shows
the identity's attribution defaults for descendant checkouts, not unrelated
current Git defaults.

Token environment conflicts, SSH command overrides, and active author or
committer environment overrides are reported without printing credentials.
It does not perform network authentication, prove an SSH alias's account, or
guarantee that Git credential overrides cannot change the configured behavior.
`--list` is unchanged; no `--list-identities` flag has been added.

## Storage and routing

An identity is a single gh-owned Git configuration file:

```gitconfig
# ~/.config/gh/identities/work.gitconfig (location follows GH_CONFIG_DIR)
[gh-identity "github.com"]
    name = work
    account = alice
    sshHost = github-work
    gitUserName = Alice Example
    gitUserEmail = alice@work.example
[user]
    name = Alice Example
    email = alice@work.example
[credential "https://github.com"]
    username = alice
```

The supported `gh-identity` fields are the identity definition. `gh` regenerates
the native Git sections when editing it. Manual additions are unsupported and
will be discarded. No token, private key, or duplicate account credential is
stored in the identity file.

JSON output and saved identity metadata use `gitUserName` and `gitUserEmail`.
Earlier `gitName` and `gitEmail` metadata remains readable; editing an identity
rewrites it with the new names. Public and GitHub noreply email addresses are
preserved as supplied.

Scope choices live beside the identities directory, not inside it:

```text
GH_CONFIG_DIR/
  auth.gitconfig
  identities/
    work.gitconfig
    personal.gitconfig
```

`auth.gitconfig` is the single managed file containing both scope-to-identity
references and their native Git routing. For example, its scope records include:

```gitconfig
[gh-auth-binding "0"]
    identity = work
    host = github.com
    owner = company
[gh-auth-binding "1"]
    identity = personal
    host = github.com
    directory = /Users/alice/personal
[gh-auth-binding "2"]
    identity = work
    host = github.com
    repository = /Users/alice/work/project/.git
```

The numbered sections are generated records, not user-facing binding names.
Directory records always include descendants. gh reads these records without
evaluating includes, so it can also select an identity outside a repository.
The same file contains the generated Git configuration:

- One global Git include points to `auth.gitconfig`.
- Repository `includeIf gitdir:` conditions match the common `.git` directory
  exactly and its `worktrees/*` entries. Directory conditions match `/**/.git`
  and `/**/.git/worktrees/*`, so outside worktrees inherit from their main repo.
- Native `includeIf hasconfig:remote.*.url:` conditions implement owner bindings.
- Includes are ordered owner, shallower trees, deeper trees, then repository.
- A host-scoped credential-helper reset requires this `gh` executable as the
  helper and enables `useHttpPath` so owner selection has the repository path.
- A managed `core.sshCommand` invokes this executable's hidden `auth git-ssh`
  dispatcher. The dispatcher selects the identity using the same scope rules
  as API and credential selection, then delegates to the user's existing
  connection command with the selected alias. It does not edit SSH configuration.

Scope records and Git routing are replaced together in one atomic file write.
There is no JSON scope store. Earlier prototype run directories containing
`identities/bindings.json` or `identities/bindings.gitconfig` are rejected with
guidance rather than silently ignored or migrated. Start a fresh scenario run
instead of reusing those directories. For a manually configured prototype,
remove its old scope files and their global Git include before reapplying
choices with `gh auth use`; reusable identity definitions can be retained.

There are deliberately **no generated `url.*.insteadOf` rules**. Those rules
accumulate across includes and choose the longest matching prefix, making
overlapping identity profiles unsafe. The dispatcher keeps routing a
single-valued choice without changing canonical remote URLs.

The built binary's path is persistent configuration. Moving/removing it breaks
the fixture's credential/SSH helpers until a binding is applied again.
Compatible global `core.sshCommand` settings are retained and used underneath
the dispatcher. Repository-local commands take precedence over a global
dispatcher, so persistent directory setup rejects those without changing them.
Applying an HTTPS-only identity to a repository with an
existing matching SSH remote is rejected with setup guidance.
Environment tokens are rejected for implicit identity selection because their
account is unknown. Explicit `--account` instead selects the named saved
credential, as described above.

API authentication uses the common refresh-aware HTTP transport. It resolves
REST `/repos/{owner}/...` and `/orgs/{owner}/...` contexts, common GraphQL owner
variables, and literal GraphQL `owner:` arguments. It uses saved per-account
credentials and the existing refresh machinery, without changing the globally
active account. Unbound requests retain existing behavior.

## SSH command cooperation

The dispatcher chooses the SSH alias; the user's command makes the connection.
For example, an existing command can obtain an enterprise certificate before
launching SSH:

```gitconfig
[core]
    sshCommand = enterprise-ssh -F company.conf
[ssh]
    variant = ssh
```

`auth use` leaves those entries in place. It installs its managed include and
checks that Git actually selects the dispatcher. If include ordering prevents
that, the attempted binding and newly added include are rolled back rather
than claiming success or replacing the user's command.

On each connection, the dispatcher reads the applicable Git settings, excludes
only its own managed entries, and delegates to the underlying command. Edits to
that original command take effect without refreshing a saved copy. Removing the
last binding removes the managed SSH command and reveals the original settings
again. Changes to gh's own managed SSH entries are rejected rather than silently
overwritten.

For persistent use, the delegate is the underlying `core.sshCommand`, then
`GIT_SSH`, then `ssh` on PATH. `GIT_SSH` is treated as a literal executable path,
not a shell command. `ssh` and `ssh.exe` names are recognized case-insensitively;
other names require an explicit OpenSSH variant declaration. This declaration
does not verify that a wrapper uses the expected key or honors the destination.

`GIT_SSH_COMMAND` bypasses Git configuration entirely, so persistent setup
refuses it without changing it. Repository-local, worktree, and command-level
SSH commands can also bypass the global dispatcher; they are not silently
wrapped persistently. A one-off `--account` invocation can instead preserve
these compatible commands underneath its invocation-local wrapper, including
the original `GIT_SSH_COMMAND` and `GIT_SSH_VARIANT`.

Both modes reject unsupported variants such as Plink, unsupported shell
expressions, and recursive dispatcher configurations. There is no arbitrary
shell-command or Windows/WSL-interoperability guarantee. The executable lookup
still uses `safeexec.LookPath`, consistent with gh's existing SSH translator.
The cooperation tests use isolated Unix executables and local Git repositories;
they are not live enterprise authentication or Windows qualification.

## Recorded issues and design decisions to revisit

These are not claims of production readiness:

1. **Declared SSH route is not account verification.** The alias can change
   externally; a key can authenticate as another account or stop working.
   Commands explicitly describe the route as unverified. The live runner checks
   its temporary aliases against GitHub; no real SSH authentication has been
   attempted by the agent. The prototype establishes
   consistent selection/routing, not proof of the server-authenticated account.
2. **Clone uses invocation scope.** Standalone Git does not give its SSH
   subprocess the destination path. Clone authenticates using the caller's
   repository/tree binding, then target-owner fallback. Both Git and gh follow
   that rule. A different destination binding takes effect for later commands.
   Whether that transition needs explicit confirmation is an open UX question.
3. **Optional metadata inherits ordinary Git configuration.** A repository's
   account-only identity can inherit name/email from a broader tree/owner
   include. Replacing a binding removes that binding's old metadata, but does
   not erase settings contributed by other scopes. `inherited-metadata`
   makes the distinction visible rather than pretending includes are profiles.
4. **Native Git overrides still exist.** Repository-local `user.*`,
   `credential.*`, `core.sshCommand`, command-line `git -c`, `GIT_SSH_COMMAND`,
   explicit Authorization headers, and author/committer environment settings
   can override generated configuration. `auth use` rejects `GIT_SSH_COMMAND`
   at application time but cannot prevent future independent Git
   invocations from overriding it. Inspection is not yet a full diagnostic of
   these conflicts.
5. **Multiple owners/hosts and metadata.** Authentication can select by target
   owner, but a commit has only one name/email pair. A repository with multiple
   differently bound owners should get an explicit repository identity;
   context-free inspection rejects ambiguous owner selection. Multiple-host
   metadata conflicts and arbitrary GraphQL node-ID-only operations need more
   design. Implicit GraphQL requests spanning different owners are rejected
   rather than guessing; `--account` supplies an explicit account for the request.
6. **Git-dir scope limitations.** Linked worktrees are supported through their
   main repository's common Git directory. Bare-main repositories, submodules
   and repositories using them, separate Git directories, and Windows
   persistence remain unsupported. Path patterns containing glob metacharacters
   are rejected. Different-host overlapping repository/directory/owner choices
   cannot supply host-specific Git attribution.
7. **Configuration lifecycle.** Individual files are atomically replaced, but
   concurrent binding writers and multi-file crash recovery are not hardened.
   Removing all bindings cleans up the managed integration and its global
   include. Identity names, hostnames, and account references are validated.
8. **Other account UX is unchanged.** `gh auth status`, `gh auth switch`, and
   `gh auth token` retain their existing global/explicit-account meanings.
   Use `gh auth use` to inspect the scoped selection. Custom clients not using
   the common refresh-aware HTTP transport need separate integration.

## Local validation

The following checks passed during development. They are local evidence, not
proof of live authentication or cross-platform compatibility. The guided live
runner and live acceptance tests require separate authorization.

| Command | Result |
| --- | --- |
| `GOTOOLCHAIN=local make` | Passed; built this worktree's `bin/gh`. |
| `go test ./script/identity-e2e -run '^TestOffline' -count=1 -args -gh "$PWD/bin/gh"` | Passed: 13 independent journeys, including main/existing/future worktree inheritance and family cleanup, native commits, simulated HTTPS/SSH authentication, and configuration preservation. |
| `go test -race ./pkg/cmd/auth/identity ./pkg/cmd/auth/use ./pkg/cmd/auth/logout ./internal/identity` | Passed, including typed deletion confirmation, cancellation, locked-config rollback, preservation of unexpected settings, and missing-account/logout guidance with a fake keyring. |
| `go test -race ./internal/identity ./pkg/cmd/auth/use ./pkg/cmd/auth/gitcredential ./pkg/cmd/repo/create` | Passed. |
| `go test -race ./pkg/cmd/root ./internal/identity ./internal/config ./pkg/cmd/auth/gitcredential` | Passed, including actual automatic PR pushes through isolated HTTPS and SSH helpers. Disabling shared Git override propagation made both PR journeys fail, confirming they detect API-only overrides. |
| `GOTOOLCHAIN=local go test ./pkg/cmd/auth/... ./pkg/cmd/root ./pkg/cmd/repo/clone` | Passed. |
| `GOTOOLCHAIN=local go test ./api -run '^(TestRefreshAwareAuthenticationUsesBoundIdentity\|TestCloneRepositoryLookupUsesOwnerIdentity\|TestNewHTTPClientRejectsConfigAndRefreshingConfig)$'` | Passed; mocked REST and the actual clone GraphQL lookup select the bound account without sockets. |
| `GOTOOLCHAIN=local go vet ./internal/identity ./pkg/cmd/auth/... ./api` | Passed. |
| `make lint` | Passed with `0 issues.` using Go 1.27.3. |
| `go test ./...` | Passed after adversarial hardening, with authentication environment tokens unset. This is the full ordinary suite, not live acceptance. |
| `go test -race -tags=ghdebug ./internal/keyring ./internal/config ./internal/identity ./pkg/cmd/auth/identity ./pkg/cmd/auth/use ./pkg/cmd/auth/logout ./pkg/cmd/auth/gitcredential ./pkg/cmd/root ./api` | Passed, including fake-keyring preservation, real-config gist/gateway routing, Unicode Git config, cleanup, and unsupported-layout refusal. |
| `bash -n script/identity-e2e/run.sh script/identity-e2e/bootstrap.sh` | Passed. |
| `shellcheck -x script/identity-e2e/run.sh` | Passed, including sourced bootstrap functions. |
| `go test ./script/identity-e2e -run '^TestRunner' -count=1` | Passed: all 20 original scenarios using fake credentials, including keyring-isolation capability gating, no-refresh token capture, guarded scenario login, and confirmation forwarding; no GitHub resources. |
