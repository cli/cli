---
name: writing-acceptance-tests
description: Use when adding or changing GitHub CLI acceptance tests, txtar scripts, testdata groups, repository fixtures, or acceptance harness behavior.
---

# Writing Acceptance Tests

Acceptance tests exercise `gh` against live GitHub resources. Minimize repository
creation and cloning without allowing concurrent scripts to interfere.

Read `acceptance/README.md` and nearby scripts before editing.

## Declare token capability

Every `.txtar` script must start with exactly one capability declaration:

```txtar
# requires-user-capability: false
```

Set the declaration to `true` only when the script requires a user principal,
such as account SSH/GPG keys, personal-account forks, or user membership APIs.
Repository and organization operations supported by an installation token
should use `false`.

## Choose one repository mode

Every script must contain exactly one declaration:

| Mode | Use when |
| --- | --- |
| `fixture-repo shared REPO` | One initialized private repository is enough and every operation can coexist with concurrent and accumulated state. |
| `fixture-repo isolated REPO` | One initialized private repository is enough, but the test needs clean state, changes repository-global state, or performs multiple coordinated Git ref updates. |
| `fixture-repo none` | No repository is needed, or the test needs multiple repositories, public visibility, special creation options, or repository lifecycle coverage. |

Managed fixtures are initialized, have discussions enabled, and are deleted by
the harness. Scripts using `shared` or `isolated` must not run `gh repo create`.

With `none`, create every required repository explicitly and immediately
register deferred cleanup. Use `defer gh repo delete --yes $ORG/$REPO` when the
repository should still exist under that name. Use `defer cleanup-repo $REPO`
when testing deletion or renaming; it is scoped to `$ORG` and treats an already
absent repository as success.

Repositories created with an initial commit can remain busy after `gh repo
create` returns. Before renaming one, use `wait-for-repository-ready
$ORG/$REPO` to wait for its default-branch commit. GitHub can retain the
repository creation lock after that commit becomes readable, so keep the
`gh repo rename` command inline and allow a 10-second stabilization delay
between this readiness check and that command.

## Shared fixture contract

Scripts within a group run concurrently. A shared script must remain correct
when the repository already contains unrelated resources.

- Never rename, transfer, archive, delete, or globally reconfigure the
  repository. Do not toggle features, change its description, or mutate its
  default branch.
- Give resources unique, reasonably short names using `$RANDOM_STRING`, adding
  `$SCRIPT_NAME` only when useful. Width-constrained commands such as
  `gh pr status` truncate long titles.
- Make commits unique across concurrent scripts. Unique branch names do not
  affect commit IDs, so include the script's `$RANDOM_STRING` in the commit
  contents or message when scripts could otherwise create the same tree from the
  same parent.
- Consolidate related repository-scoped operations, such as merging pull
  requests, into one `isolated` script. Unique refs do not prevent concurrent
  Git pushes and merges in a shared repository from contending.
- GitHub normalizes some identifiers. Use `env2upper` for Actions variables and
  secrets whose generated names are asserted later.
- Capture the created resource's URL or ID. Filter and paginate list operations;
  never select the first, latest, or only result.
- Target the fixture with `--repo $ORG/$REPO` or `GH_REPO=$ORG/$REPO`. Clone only
  when local Git behavior is part of the test.

If any operation violates this contract, use `isolated`; do not weaken
assertions to make sharing appear safe.

## API request budgets

Scripts share token-wide API rate limits. Count requests across the whole test
process and combine compatible live assertions. Keep coverage for a narrowly
limited endpoint in one script so concurrent scripts cannot burst the limit.
For Code Search's 10 requests/minute bucket, use at most five HTTP requests in
the entire acceptance process, even when they run sequentially. This reserves
half the bucket for pagination, retries, and other token activity. Count the
requests made by each command, keep representative live coverage, and move
remaining variants to unit tests.

Use bounded condition-based waits for asynchronously registered resources.
Fixed sleeps are both slower when registration is fast and unreliable when it
is slow. Use `wait-for-workflow` after pushing a new workflow definition, then
use `wait-for-run` before watching or inspecting a triggered workflow run.
After `gh workflow run`, `wait-for-run` captures the returned run URL when the
server provides one and falls back to polling for older servers.
On timeout, `wait-for-run` captures the pushed commit and remote ref, workflow
files, recent unfiltered runs, commit check suites, and an Actions API request
ID. Use that evidence to distinguish event ingestion, run indexing, filtering,
and push/setup failures before changing fixture isolation or timeout budgets.
Cancellation tests must use a self-contained workflow with a deliberately long
step so the run cannot finish before the cancellation request, plus a short job
timeout so a failed cancellation cannot run for the full step. After
registration, use `wait-for-run-status RUN_ID in_progress` before cancellation
because the cancellation endpoint can reject a run that is still starting. Do
not wait for GitHub to finish cancellation after the command confirms that the
request was submitted; that tests Actions' eventual behavior rather than the
CLI contract. Do not lengthen registration sleeps to compensate for a short or
externally dependent job.

## Organization safety

Every live mutation must resolve through `$ORG/$REPO`, a resource ID or URL
captured from something created under `$ORG`, or fixture cleanup scoped to
`GH_ACCEPTANCE_ORG`. Do not rely on ambient Git remotes or repository context.

## Example

```txtar
env2upper VAR_NAME=TESTSCRIPTS_${RANDOM_STRING}
fixture-repo shared REPO
env GH_REPO=$ORG/$REPO

exec gh variable set $VAR_NAME --body value
exec gh variable get $VAR_NAME
stdout '^value$'
```

## Validate

Run metadata checks without live credentials:

```sh
go test -tags=acceptance \
  -run '^(TestSelectAcceptanceTestGroups|TestFilterAcceptanceScripts|TestTokenHasUserCapability|TestAcceptanceScriptsDeclareUserCapabilityRequirement|TestAcceptanceScriptsDeclareFixtureRepository|TestRequiresUserCapabilityForScript|TestValidateFixtureRepositoryDeclaration|TestFixtureRepositoryManager)$' \
  ./acceptance
```

Run a changed live script with `GH_ACCEPTANCE_GROUP` and
`GH_ACCEPTANCE_SCRIPT`; see `acceptance/README.md` for the credential variables.
Use `-count=1` to bypass the Go test cache. Start with one script. If concurrency
is required to reproduce the behavior, pass only the contending scripts as a
comma-separated filter and repeat that focused set before widening the run.
