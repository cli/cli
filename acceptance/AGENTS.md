# Acceptance Test Instructions

Acceptance tests exercise `gh` against live GitHub resources. Keep them isolated
and safe to rerun.

## Adding or Changing Tests

- Put each script in `testdata/<group>/`. Groups are discovered automatically;
  do not register them in the Go harness.
- Every script must contain exactly one repository fixture declaration:

  ```txtar
  fixture-repo shared REPO
  fixture-repo isolated REPO
  fixture-repo none
  ```

- Use `shared` when the test can coexist with concurrent and accumulated state
  in one initialized private repository. Use unique resource names, capture IDs
  from creation output, filter queries to the resource under test, and paginate
  whenever the desired resource may fall outside the first page. Never assume a
  resource is first, latest, or the only result.
- Shared tests must not rename, archive, transfer, delete, or globally
  reconfigure the repository. Avoid default-branch mutations and other state
  that can affect concurrent users.
- Use `isolated` when the test needs one initialized private repository with
  clean state or needs to modify repository-global state.
- Use `none` when the test needs no repository, multiple repositories, public
  visibility, special creation options, or explicitly tests repository
  lifecycle commands. Such tests own their repository lifecycle and must
  register cleanup with `defer` immediately after creating each resource. Use
  `defer cleanup-repo NAME` when the test may rename or delete the repository;
  this cleanup treats an already absent repository as success.
- Scripts using `shared` or `isolated` must not invoke `gh repo create`. The
  harness lazily creates their repository, stores its bare name in the requested
  environment variable, and deletes all managed repositories after the suite.
- Give unmanaged resources and resources inside shared repositories unique
  names using `$SCRIPT_NAME` and `$RANDOM_STRING`.

## Running Tests

Run all acceptance tests with the required live-test environment:

```sh
GH_ACCEPTANCE_HOST=github.com \
GH_ACCEPTANCE_ORG=<organization> \
GH_ACCEPTANCE_TOKEN=<token> \
go test -tags=acceptance ./acceptance
```

Set `GH_ACCEPTANCE_GROUP=<group>` to run one group. Set
`GH_ACCEPTANCE_SCRIPT=<file.txtar>` as well to run one script in that group.

Run the metadata and group-selection checks without live credentials:

```sh
go test ./acceptance
go test -tags=acceptance \
  -run '^(TestSelectAcceptanceTestGroups|TestAcceptanceScriptsDeclareFixtureRepository|TestValidateFixtureRepositoryDeclaration|TestFixtureRepositoryManager)$' \
  ./acceptance
```
