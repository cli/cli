## Acceptance Tests

The acceptance tests are blackbox* tests that are expected to interact with resources on a real GitHub instance.

*Note: they aren't strictly blackbox because `exec gh` commands delegate to a binary set up by `testscript` that calls into `ghcmd.Main`. However, since our real `func main` is an extremely thin adapter over `ghcmd.Main`, this is reasonable. This tradeoff avoids us building the binary ourselves for the tests, and allows us to get code coverage metrics.

### Running the Acceptance Tests

The acceptance tests currently require three environment variables to be set:
 * `GH_HOST`
 * `GH_ACCEPTANCE_ORG`
 * `GH_TOKEN`

While `GH_HOST` may not strictly be necessary because `gh` can choose a host, it is required to avoid ambiguity and unexpected results depending on the state of `gh`.

The `GH_ACCEPTANCE_ORG` is an organization that the tests can manage resources in.

The `GH_TOKEN` must already have the necessary scopes for each test, and must have permissions to act in the `GH_ACCEPTANCE_ORG`. See [Effective Test Authoring](#effective-test-authoring) for how tests must handle tokens without sufficient scopes.

The acceptance tests have a build constraint of `//go:build acceptance`, this means that `go test ./...` will continue to work without any modifications. The `acceptance` tag must therefore be provided when running `go test`.

A full example invocation can be found below:

```
GH_HOST=<host> GH_ACCEPTANCE_ORG=<org> GH_TOKEN=<token> test -tags=acceptance ./acceptance
```

### Acceptance Test VS Code Support

Due to the `//go:build acceptance` build constraint, some functionality is limited because `gopls` isn't being informed about the tag. To resolve this, set the following in your `settings.json`:

```json
  "gopls": {
    "buildFlags": [
        "-tags=acceptance"
    ]
  },
```

### Effective Test Authoring

This section will likely extend over time.

#### Test Isolation

#### Scope Validation

#### Limit Custom Commands

...

### Further Reading

https://bitfieldconsulting.com/posts/test-scripts

https://atlasgo.io/blog/2024/09/09/how-go-tests-go-test

https://encore.dev/blog/testscript-hidden-testing-gem
