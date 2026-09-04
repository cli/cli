---
name: writing-acceptance-tests
description: Use when adding or changing GitHub CLI acceptance tests, txtar scripts, testdata groups, repository fixtures, or acceptance harness behavior.
---

# Writing Acceptance Tests

Acceptance tests exercise `gh` against live GitHub resources. Minimize repository
creation and cloning without allowing concurrent scripts to interfere.

Read `acceptance/README.md` and nearby scripts before editing. Test groups are
discovered from `acceptance/testdata/<group>/`; do not register them in the Go
harness, workflow, or dispatch helper.

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
| `fixture-repo isolated REPO` | One initialized private repository is enough, but the test needs clean state or changes repository-global state. |
| `fixture-repo none` | No repository is needed, or the test needs multiple repositories, public visibility, special creation options, or repository lifecycle coverage. |

Managed fixtures are initialized, have discussions enabled, and are deleted by
the harness. Scripts using `shared` or `isolated` must not run `gh repo create`.

With `none`, create every required repository explicitly under `$ORG` and
immediately register `defer cleanup-repo $REPO`. The cleanup is idempotent when a
test deletes or renames the repository itself.

## Shared fixture contract

Scripts within a group run concurrently. A shared script must remain correct
when the repository already contains unrelated resources.

- Never rename, transfer, archive, delete, or globally reconfigure the
  repository. Do not toggle features, change its description, or mutate its
  default branch.
- Give resources unique, reasonably short names using `$RANDOM_STRING`, adding
  `$SCRIPT_NAME` only when useful. Width-constrained commands such as
  `gh pr status` truncate long titles.
- GitHub normalizes some identifiers. Use `env2upper` for Actions variables and
  secrets whose generated names are asserted later.
- Capture the created resource's URL or ID. Filter and paginate list operations;
  never select the first, latest, or only result.
- Target the fixture with `--repo $ORG/$REPO` or `GH_REPO=$ORG/$REPO`. Clone only
  when local Git behavior is part of the test.

If any operation violates this contract, use `isolated`; do not weaken
assertions to make sharing appear safe.

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
  -run '^(TestSelectAcceptanceTestGroups|TestTokenHasUserCapability|TestAcceptanceScriptsDeclareUserCapabilityRequirement|TestAcceptanceScriptsDeclareFixtureRepository|TestRequiresUserCapabilityForScriptErrors|TestValidateFixtureRepositoryDeclaration|TestFixtureRepositoryManager)$' \
  ./acceptance
```

Run a changed live script with `GH_ACCEPTANCE_GROUP` and
`GH_ACCEPTANCE_SCRIPT`; see `acceptance/README.md` for the credential variables.
