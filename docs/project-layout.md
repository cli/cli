# GitHub CLI project layout

At a high level, these areas make up the `github.com/cli/cli` project:
- [`cmd/`](../cmd) - `main` packages for building binaries such as the `gh` executable
- [`pkg/`](../pkg) - most other packages, including the implementation for individual gh commands
- [`docs/`](../docs) - documentation for maintainers and contributors
- [`script/`](../script) - build and release scripts
- [`internal/`](../internal) - Go packages highly specific to our needs and thus internal
- [`go.mod`](../go.mod) - external Go dependencies for this project, automatically fetched by Go at build time

Some auxiliary Go packages are at the top level of the project for historical reasons:
- [`api/`](../api) - main utilities for making requests to the GitHub API
- [`context/`](../context) - DEPRECATED: use only for referencing git remotes
- [`git/`](../git) - utilities to gather information from a local git repository
- [`test/`](../test) - DEPRECATED: do not use
- [`utils/`](../utils) - DEPRECATED: use only for printing table output

## Command-line help text

Running `gh help issue list` displays help text for a topic. In this case, the topic is a specific command,
and help text for every command is embedded in that command's source code. The naming convention for gh
commands is:
```
pkg/cmd/<command>/<subcommand>/<subcommand>.go
```
Following the above example, the main implementation for the `gh issue list` command, including its help
text, is in [pkg/cmd/issue/view/view.go](../pkg/cmd/issue/view/view.go)

Other help topics not specific to any command, for example `gh help environment`, are found in
[pkg/cmd/root/help_topic.go](../pkg/cmd/root/help_topic.go).

During our release process, these help topics are [automatically converted](../cmd/gen-docs/main.go) to
manual pages and published under https://cli.github.com/manual/.

## How GitHub CLI works

To illustrate how GitHub CLI works in its typical mode of operation, let's build the project, run a command,
and talk through which code gets run in order.

1. `go run script/build.go` - Makes sure all external Go depedencies are fetched, then compiles the
   `cmd/gh/main.go` file into a `bin/gh` binary.
2. `bin/gh issue list --limit 5` - Runs the newly built `bin/gh` binary (note: on Windows you must use
   backslashes like `bin\gh`) and passes the following arguments to the process: `["issue", "list", "--limit", "5"]`.
3. `func main()` inside `cmd/gh/main.go` is the first Go function that runs. The arguments passed to the
   process are available through `os.Args`.
4. The `main` package initializes the "root" command with `root.NewCmdRoot()` and dispatches execution to it
   with `rootCmd.ExecuteC()`.
5. The [root command](../pkg/cmd/root/root.go) represents the top-level `gh` command and knows how to
   dispatch execution to any other gh command nested under it.
6. Based on `["issue", "list"]` arguments, the execution reaches the `RunE` block of the `cobra.Command`
   within [pkg/cmd/issue/list/list.go](../pkg/cmd/issue/list/list.go).
7. The `--limit 5` flag originally passed as arguments be automatically parsed and its value stored as
   `opts.LimitResults`.
8. `func listRun()` is called, which is responsible for implementing the logic of the `gh issue list` command.
9. The command collects information from sources like the GitHub API then writes the final output to
   standard output and standard error [streams](../pkg/iostreams/iostreams.go) available at `opts.IO`.
10. The program execution is now back at `func main()` of `cmd/gh/main.go`. If there were any Go errors as a
    result of processing the command, the function will abort the process with a non-zero exit status.
    Otherwise, the process ends with status 0 indicating success.

## How to add a new command

0. First, check on our issue tracker to verify that our team had approved the plans for a new command.
1. Create a package for the new command, e.g. for a new command `gh boom` create the following directory
   structure: `pkg/cmd/boom/`
2. The new package should expose a method, e.g. `NewCmdBoom()`, that accepts a `*cmdutil.Factory` type and
   returns a `*cobra.Command`.
   * Any logic specific to this command should be kept within the command's package and not added to any
     "global" packages like `api` or `utils`.
3. Use the method from the previous step to generate the command and add it to the command tree, typically
   somewhere in the `NewCmdRoot()` method.

## How to write tests

This task might be tricky. Typically, gh commands do things like look up information from the git repository
in the current directory, query the GitHub API, scan the user's `~/.ssh/config` file, clone or fetch git
repositories, etc. Naturally, none of these things should ever happen for real when running tests, unless
you are sure that any filesystem operations are stricly scoped to a location made for and maintained by the
test itself. To avoid actually running things like making real API requests or shelling out to `git`
commands, we stub them. You should look at how that's done within some existing tests.

To make your code testable, write small, isolated pieces of functionality that are designed to be composed
together. Prefer table-driven tests for maintaining variations of different test inputs and expectations
when exercising a single piece of functionality.
