## Acceptance Tests

The acceptance tests are blackbox* tests that are expected to interact with resources on a real GitHub instance.

*Note: they aren't strictly blackbox because `exec gh` commands delegate to a binary set up by `testscript` that calls into `ghcmd.Main`. However, since our real `func main` is an extremely thin adapter over `ghcmd.Main`, this is reasonable. This tradeoff avoids us building the binary ourselves for the tests, and allows us to get code coverage metrics.

### Running the Acceptance Tests

The acceptance tests have a build constraint of `//go:build acceptance`, this means that `go test ./...` will continue to work without any modifications. The `acceptance` tag must therefore be provided when running `go test`.

The following environment variables are required:

#### `GH_ACCEPTANCE_HOST`

The GitHub host to target e.g. `github.com`

#### `GH_ACCEPTANCE_ORG`

The organization in which the acceptance tests can manage resources in.

#### `GH_ACCEPTANCE_TOKEN`

The token to use for authenticatin with the `GH_HOST`. This must already have the necessary scopes for each test, and must have permissions to act in the `GH_ACCEPTANCE_ORG`. See [Effective Test Authoring](#effective-test-authoring) for how tests must handle tokens without sufficient scopes.

---

A full example invocation can be found below:

```
GH_ACCEPTANCE_HOST=<host> GH_ACCEPTANCE_ORG=<org> GH_ACCEPTANCE_TOKEN=<token> test -tags=acceptance ./acceptance
```

### Writing Tests

This section is to be expanded over time as we write more tests and learn more.

#### Environment Variables

The following custom environment variables are made available to the scripts:
 * `GH_HOST`: Set to value of the `GH_ACCEPTANCE_ORG` env var provided to `go test`
 * `ORG`: Set to the value of the `GH_ACCEPTANCE_ORG` env var provided to `go test`
 * `GH_TOKEN`: Set to the value of the `GH_ACCEPTANCE_TOKEN` env var provided to `go test`
 * `RANDOM_STRING`: Set to a length 10 random string of letters to help isolate globally visible resources
 * `SCRIPT_NAME`: Set to the name of the `testscript` currently running, without extension e.g. `pr-view`
 * `HOME`: Set to the initial working directory. Required for `git` operations
 * `GH_CONFIG_DIR`: Set to the initial working directory. Required for `gh` operations

### Acceptance Test VS Code Support

Due to the `//go:build acceptance` build constraint, some functionality is limited because `gopls` isn't being informed about the tag. To resolve this, set the following in your `settings.json`:

```json
  "gopls": {
    "buildFlags": [
        "-tags=acceptance"
    ]
  },
```

You can install the `txtar` or `vscode-testscript` extensions to get syntax highlighting.

### Debugging Tests

When tests fail they fail like this:

```
➜ go test -tags=acceptance ./acceptance
--- FAIL: TestPullRequests (0.00s)
    --- FAIL: TestPullRequests/pr-merge (11.07s)
        testscript.go:584: WORK=/private/var/folders/45/sdnm1hp10nj1s9q57dp3bc5h0000gn/T/go-test-script2778137936/script-pr-merge
            # Use gh as a credential helper (0.693s)
            # Create a repository with a file so it has a default branch (1.155s)
            # Defer repo cleanup (0.000s)
            # Clone the repo (1.551s)
            # Prepare a branch to PR with a single file (1.168s)
            # Create the PR (1.903s)
            # Check that the file doesn't exist on the main branch (0.059s)
            # Merge the PR (2.426s)
            # Check that the state of the PR is now merged (0.571s)
            # Pull and check the file exists on the main branch (1.074s)
            # And check we had a merge commit (0.462s)
            > exec git show HEAD
            [stdout]
            commit 85d32c1a83ace270f6754c61f3f7e14956be0a47
            Author: William Martin <williammartin@william-github-laptop.kpn>
            Date:   Fri Oct 11 15:23:56 2024 +0200

                Add file.txt

            diff --git a/file.txt b/file.txt
            new file mode 100644
            index 0000000..7449899
            --- /dev/null
            +++ b/file.txt
            @@ -0,0 +1 @@
            +Unimportant contents
            > stdout 'Merge pull request #1'
            FAIL: testdata/pr/pr-merge.txtar:42: no match for `Merge pull request #1` found in stdout
```

This is generally enough information to understand why a test has failed. However, we can get more information by providing the `-v` flag to `go test`, which turns on verbose mode and shows each command and any associated `stdio`.

> [!WARNING]
> Verbose mode dumps the `testscript` environment variables, including the `GH_TOKEN`, so be careful.

Finally, the `testscript.Params` struct has a `TestWork` field; when `TestWork` is set to true, the `WORK` directory will not be cleaned up, allowing for manual investigation of state in the shell. However, `defer` statements will still be run.

TODO: Probably we want to add a `NO_CLEANUP` flag so `defer` is not run. Maybe this implies we want `defer` to be called `cleanup`? It might not be so clear in its intent though. Maybe `NO_CLEANUP_WORK` and `NO_DEFER`?

### Effective Test Authoring

This section is to be expanded over time as we write more tests and learn more.

#### Test Isolation

The `testscript` library creates a somewhat isolated environment for each script. Each script gets a directory with limited environment variables by default. As far as reasonable, we should look to write scripts that depend on nothing more than themselves, the GitHub resources they manage, and limited additional environmental injection from our own `testscript` setup.

Here are some guidelines around test isolation:
 * Favour duplication in test setup over abstracting a new `testscript` command
 * Favour a `testscript` owning an entire resource lifecycle over shared resource until we see a performance or rate limiting issue
 * Use the `RANDOM_STRING` env var for globally visible resources to avoid conflicts

### Debris

Since these scripts are creating resources on a GitHub instance, we should try our best to cleanup after them. Use the `defer` keyword to ensure a command runs at the end of a test even in the case of failure.

#### Scope Validation

TODO: I believe tests should early exit if the correct scopes aren't in place to execute the entire lifecycle. It's extremely annoying if a `defer` fails to clean up resources because there's no `delete_repo` scope for example. However, I'm not sure yet whether this scope checking should be in the Go tests or in the scripts themselves. It seems very cool to understand required scopes for a script just by looking at the script itself.

### Further Reading

https://bitfieldconsulting.com/posts/test-scripts

https://atlasgo.io/blog/2024/09/09/how-go-tests-go-test

https://encore.dev/blog/testscript-hidden-testing-gem
