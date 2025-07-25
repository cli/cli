Thanks for helping us build Boulder! This page contains requirements and
guidelines for Boulder contributions.

# Patch Requirements

* All new functionality and fixed bugs must be accompanied by tests.
* All patches must meet the deployability requirements listed below.
* We prefer pull requests from external forks be created with the ["Allow edits
  from
  maintainers"](https://github.com/blog/2247-improving-collaboration-with-forks)
  checkbox selected.

# Review Requirements

* All pull requests must receive at least one approval by a [CODEOWNER](../CODEOWNERS) other than the author. This is enforced by GitHub itself.
* All pull requests should receive at least two approvals by [Trusted Contributors](https://github.com/letsencrypt/cp-cps/blob/main/CP-CPS.md#161-definitions).
  This requirement may be waived when:
  * the change only modifies documentation;
  * the change only modifies tests;
  * in exceptional circumstances, such as when no second reviewer is available at all.

  This requirement should not be waived when:
  * the change is not written by a Trusted Contributor, to ensure that at least two TCs have eyes on it.
* New commits pushed to a branch invalidate previous reviews. In other words, a
  reviewer must give positive reviews of a branch after its most recent pushed
  commit.
* If a branch contains commits from multiple authors, it needs a reviewer who
  is not an author of commits on that branch.
* Review changes to or addition of tests just as rigorously as you review code
  changes. Consider: Do tests actually test what they mean to test? Is this the
  best way to test the functionality in question? Do the tests cover all the
  functionality in the patch, including error cases?
* Are there new RPCs or config fields? Make sure the patch meets the
  Deployability rules below.

# Merge Requirements

We have a bot that will comment on some PRs indicating there are:

 1. configuration changes
 2. SQL schema changes
 3. feature flag changes

These may require either a CP/CPS review or filing of a ticket to make matching changes
in production. It is the responsibility of the person merging the PR to make sure
the required action has been performed before merging. Usually this will be confirmed
in a comment or in the PR description.

# Patch Guidelines

* Please include helpful comments. No need to gratuitously comment clear code,
  but make sure it's clear why things are being done. Include information in
  your pull request about what you're trying to accomplish with your patch.
* Avoid named return values. See
  [#3017](https://github.com/letsencrypt/boulder/pull/3017) for an example of a
  subtle problem they can cause.
* Do not include `XXX`s or naked `TODO`s. Use
  the formats:

  ```go
  // TODO(<email-address>): Hoverboard + Time-machine unsupported until upstream patch.
  // TODO(#<num>): Pending hoverboard/time-machine interface.
  // TODO(@githubusername): Enable hoverboard kickflips once interface is stable.
  ```

# Squash merging

Once a pull request is approved and the tests are passing, the author or any
other committer can merge it. We always use [squash
merges](https://github.com/blog/2141-squash-your-commits) via GitHub's web
interface. That means that during the course of your review you should
generally not squash or amend commits, or force push. Even if the changes in
each commit are small, keeping them separate makes it easier for us to review
incremental changes to a pull request. Rest assured that those tiny changes
will get squashed into a nice meaningful-size commit when we merge.

If the CI tests are failing on your branch, you should look at the logs
to figure out why. Sometimes (though rarely) they fail spuriously, in which
case you can post a comment requesting that a project owner kick the build.

# Error handling

All errors must be addressed in some way: That may be simply by returning an
error up the stack, or by handling it in some intelligent way where it is
generated, or by explicitly ignoring it and assigning to `_`. We use the
`errcheck` tool in our integration tests to make sure all errors are
addressed. Note that ignoring errors, even in tests, should be rare, since
they may generate hard-to-debug problems.

When handling errors, always do the operation which creates the error (usually
a function call) and the error checking on separate lines:
```
err := someOperation(args)
if err != nil {
  return nil, fmt.Errorf("some operation failed: %w", err)
}
```
We avoid the `if err := someOperation(args); err != nil {...}` style as we find
it to be less readable and it can give rise to surprising scoping behavior.

We define two special types of error. `BoulderError`, defined in
errors/errors.go, is used specifically when an typed error needs to be passed
across an RPC boundary. For instance, if the SA returns "not found", callers
need to be able to distinguish that from a network error. Not every error that
may pass across an RPC boundary needs to be a BoulderError, only those errors
that need to be handled by type elsewhere. Handling by type may be as simple as
turning a BoulderError into a specific type of ProblemDetail.

The other special type of error is `ProblemDetails`. We try to treat these as a
presentation-layer detail, and use them only in parts of the system that are
responsible for rendering errors to end-users, i.e. WFE2. Note
one exception: The VA RPC layer defines its own `ProblemDetails` type, which is
returned to the RA and stored as part of a challenge (to eventually be rendered
to the user).

Within WFE2, ProblemDetails are sent to the client by calling
`sendError()`, which also logs the error. For internal errors like timeout,
or any error type that we haven't specifically turned into a ProblemDetail, we
return a ServerInternal error. This avoids unnecessarily exposing internals.
It's possible to add additional errors to a logEvent using `.AddError()`, but
this should only be done when there is is internal-only information to log
that isn't redundant with the ProblemDetails sent to the user. Note that the
final argument to `sendError()`, `ierr`, will automatically get added to the
logEvent for ServerInternal errors, so when sending a ServerInternal error it's
not necessary to separately call `.AddError`.

# Deployability

We want to ensure that a new Boulder revision can be deployed to the
currently running Boulder production instance without requiring config
changes first. We also want to ensure that during a deploy, services can be
restarted in any order. That means two things:

## Good zero values for config fields

Any newly added config field must have a usable [zero
value](https://tour.golang.org/basics/12). That is to say, if a config field
is absent, Boulder shouldn't crash or misbehave. If that config file names a
file to be read, Boulder should be able to proceed without that file being
read.

Note that there are some config fields that we want to be a hard requirement.
To handle such a field, first add it as optional, then file an issue to make
it required after the next deploy is complete.

In general, we would like our deploy process to be: deploy new code + old
config; then immediately after deploy the same code + new config. This makes
deploys cheaper so we can do them more often, and allows us to more readily
separate deploy-triggered problems from config-triggered problems.

## Flag-gating features

When adding significant new features or replacing existing RPCs the
`boulder/features` package should be used to gate its usage. To add a flag, a
new field of the `features.Config` struct should be added. All flags default
to false.

In order to test if the flag is enabled elsewhere in the codebase you can use
`features.Get().ExampleFeatureName` which gets the `bool` value from a global
config.

Each service should include a `map[string]bool` named `Features` in its
configuration object at the top level and call `features.Set` with that map
immediately after parsing the configuration. For example to enable
`UseNewMetrics` and disable `AccountRevocation` you would add this object:

```json
{
    ...
    "features": {
        "UseNewMetrics": true,
        "AccountRevocation": false,
    }
}
```

Feature flags are meant to be used temporarily and should not be used for
permanent boolean configuration options.

### Deprecating a feature flag

Once a feature has been enabled in both staging and production, someone on the
team should deprecate it:

 - Remove any instances of `features.Get().ExampleFeatureName`, adjusting code
   as needed.
 - Move the field to the top of the `features.Config` struct, under a comment
   saying it's deprecated.
 - Remove all references to the feature flag from `test/config-next`.
 - Add the feature flag to `test/config`. This serves to check that we still
   tolerate parsing the flag at startup, even though it is ineffective.
 - File a ticket to remove the feature flag in staging and production.
 - Once the feature flag is removed in staging and production, delete it from
   `test/config` and `features.Config`.

### Gating RPCs

When you add a new RPC to a Boulder service (e.g. `SA.GetFoo()`), all
components that call that RPC should gate those calls using a feature flag.
Since the feature's zero value is false, a deploy with the existing config
will not call `SA.GetFoo()`. Then, once the deploy is complete and we know
that all SA instances support the `GetFoo()` RPC, we do a followup config
deploy that sets the default value to true, and finally remove the flag
entirely once we are confident the functionality it gates behaves correctly.

### Gating migrations

We use [database migrations](https://en.wikipedia.org/wiki/Schema_migration)
to modify the existing schema. These migrations will be run on live data
while Boulder is still running, so we need Boulder code at any given commit
to be capable of running without depending on any changes in schemas that
have not yet been applied.

For instance, if we're adding a new column to an existing table, Boulder should
run correctly in three states:

1. Migration not yet applied.
2. Migration applied, flag not yet flipped.
3. Migration applied, flag flipped.

Specifically, that means that all of our `SELECT` statements should enumerate
columns to select, and not use `*`. Also, generally speaking, we will need a
separate model `struct` for serializing and deserializing data before and
after the migration. This is because the ORM package we use,
[`borp`](https://github.com/letsencrypt/borp), expects every field in a struct to
map to a column in the table. If we add a new field to a model struct and
Boulder attempts to write that struct to a table that doesn't yet have the
corresponding column (case 1), borp will fail with `Insert failed table posts
has no column named Foo`. There are examples of such models in sa/model.go,
along with code to turn a model into a `struct` used internally.

An example of a flag-gated migration, adding a new `IsWizard` field to Person
controlled by a `AllowWizards` feature flag:

```go
# features/features.go:

const (
  unused FeatureFlag = iota // unused is used for testing
  AllowWizards // Added!
)

...

var features = map[FeatureFlag]bool{
  unused: false,
  AllowWizards: false, // Added!
}
```

```go
# sa/sa.go:

struct Person {
  HatSize  int
  IsWizard bool // Added!
}

struct personModelv1 {
  HatSize int
}

// Added!
struct personModelv2 {
  personModelv1
  IsWizard bool
}

func (ssa *SQLStorageAuthority) GetPerson() (Person, error) {
  if features.Enabled(features.AllowWizards) { // Added!
    var model personModelv2
    ssa.dbMap.SelectOne(&model, "SELECT hatSize, isWizard FROM people")
    return Person{
      HatSize:  model.HatSize,
      IsWizard: model.IsWizard,
    }
  } else {
    var model personModelv1
    ssa.dbMap.SelectOne(&model, "SELECT hatSize FROM people")
    return Person{
      HatSize:  model.HatSize,
    }
  }
}

func (ssa *SQLStorageAuthority) AddPerson(p Person) (error) {
  if features.Enabled(features.AllowWizards) { // Added!
    return ssa.dbMap.Insert(context.Background(), personModelv2{
      personModelv1: {
        HatSize:  p.HatSize,
      },
      IsWizard: p.IsWizard,
    })
  } else {
    return ssa.dbMap.Insert(context.Background(), personModelv1{
      HatSize:  p.HatSize,
      // p.IsWizard ignored
    })
  }
}
```

You will also need to update the `initTables` function from `sa/database.go` to
tell borp which table to use for your versioned model structs. Make sure to
consult the flag you defined so that only **one** of the table maps is added at
any given time, otherwise borp will error.  Depending on your table you may also
need to add `SetKeys` and `SetVersionCol` entries for your versioned models.
Example:

```go
func initTables(dbMap *borp.DbMap) {
 // < unrelated lines snipped for brevity >

 if features.Enabled(features.AllowWizards) {
    dbMap.AddTableWithName(personModelv2, "person")
 } else {
    dbMap.AddTableWithName(personModelv1, "person")
 }
}
```

New migrations should be added at `./sa/db-next`:

```shell
$ cd sa/db
$ sql-migrate new -env="boulder_sa_test" AddWizards
Created migration boulder_sa/20220906165519-AddWizards.sql
```

Finally, edit the resulting file
(`sa/db-next/boulder_sa/20220906165519-AddWizards.sql`) to define your migration:

```mysql
-- +migrate Up
ALTER TABLE people ADD isWizard BOOLEAN SET DEFAULT false;

-- +migrate Down
ALTER TABLE people DROP isWizard BOOLEAN SET DEFAULT false;
```

# Expressing "optional" Timestamps
Timestamps in protocol buffers must always be expressed as
[timestamppb.Timestamp](https://pkg.go.dev/google.golang.org/protobuf/types/known/timestamppb).
Timestamps must never contain their zero value, in the sense of
`timestamp.AsTime().IsZero()`. When a timestamp field is optional, absence must
be expressed through the absence of the field, rather than present with a zero
value. The `core.IsAnyNilOrZero` function can check these cases.

Senders must check that timestamps are non-zero before sending them. Receivers
must check that timestamps are non-zero before accepting them.

# Rounding time in DB

All times that we send to the database are truncated to one second's worth of
precision. This reduces the size of indexes that include timestamps, and makes
querying them more efficient. The Storage Authority (SA) is responsible for this
truncation, and performs it for SELECT queries as well as INSERT and UPDATE.

# Release Process

The current Boulder release process is described in
[release.md](https://github.com/letsencrypt/boulder/docs/release.md). New
releases are tagged weekly, and artifacts are automatically produced for each
release by GitHub Actions.

# Dependencies

We use [go modules](https://github.com/golang/go/wiki/Modules) and vendor our
dependencies. As of Go 1.12, this may require setting the `GO111MODULE=on` and
`GOFLAGS=-mod=vendor` environment variables. Inside the Docker containers for
Boulder tests, these variables are set for you, but if you ever work outside
those containers you will want to set them yourself.

To add a dependency, add the import statement to your .go file, then run
`go build` on it. This will automatically add the dependency to go.mod. Next,
run `go mod vendor && git add vendor/` to save a copy in the vendor folder.

When vendorizing dependencies, it's important to make sure tests pass on the
version you are vendorizing. Currently we enforce this by requiring that pull
requests containing a dependency update to any version other than a tagged
release include a comment indicating that you ran the tests and that they
succeeded, preferably with the command line you run them with. Note that you
may have to get a separate checkout of the dependency (using `go get` outside
of the boulder repository) in order to run its tests, as some vendored
modules do not bring their tests with them.

## Updating Dependencies

To upgrade a dependency, [see the Go
docs](https://github.com/golang/go/wiki/Modules#how-to-upgrade-and-downgrade-dependencies).
Typically you want `go get <dependency>` rather than `go get -u
<dependency>`, which can introduce a lot of unexpected updates. After running
`go get`, make sure to run `go mod vendor && git add vendor/` to update the
vendor directory. If you forget, CI tests will catch this.

If you are updating a dependency to a version which is not a tagged release,
see the note above about how to run all of a dependency's tests and note that
you have done so in the PR.

Note that updating dependencies can introduce new, transitive dependencies. In
general we try to keep our dependencies as narrow as possible in order to
minimize the number of people and organizations whose code we need to trust.
As a rule of thumb: If an update introduces new packages or modules that are
inside a repository where we already depend on other packages or modules, it's
not a big deal. If it introduces a new dependency in a different repository,
please try to figure out where that dependency came from and why (for instance:
"package X, which we depend on, started supporting XML config files, so now we
depend on an XML parser") and include that in the PR description. When there are
a large number of new dependencies introduced, and we don't need the
functionality they provide, we should consider asking the relevant upstream
repository for a refactoring to reduce the number of transitive dependencies.

# Go Version

The [Boulder development
environment](https://github.com/letsencrypt/boulder/blob/main/README.md#setting-up-boulder)
does not use the Go version installed on the host machine, and instead uses a
Go environment baked into a "boulder-tools" Docker image. We build a separate
boulder-tools container for each supported Go version. Please see [the
Boulder-tools
README](https://github.com/letsencrypt/boulder/blob/main/test/boulder-tools/README.md)
for more information on upgrading Go versions.

# ACME Protocol Divergences

While Boulder attempts to implement the ACME specification as strictly as
possible there are places at which we will diverge from the letter of the
specification for various reasons. We detail these divergences (for both the
V1 and V2 API) in the [ACME divergences
doc](https://github.com/letsencrypt/boulder/blob/main/docs/acme-divergences.md).

# ACME Protocol Implementation Details

The ACME specification allows developers to make certain decisions as to how
various elements in the RFC are implemented. Some of these fully conformant
decisions are listed in [ACME implementation details
doc](https://github.com/letsencrypt/boulder/blob/main/docs/acme-implementation_details.md).

## Code of Conduct

The code of conduct for everyone participating in this community in any capacity
is available for reference
[on the community forum](https://community.letsencrypt.org/guidelines).

## Problems or questions?

The best place to ask dev related questions is on the [Community
Forums](https://community.letsencrypt.org/).
