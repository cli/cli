// Command ghmock runs gh binaries against a fake GitHub API.
//
// gh treats GH_HOST=github.localhost as a local development host and sends its
// API calls there over plain HTTP. ghmock points HTTP_PROXY at a server on
// localhost, so every call lands on responses ghmock controls, and it records
// each request. Routing is all it does: ghmock does not block network access,
// so it must run somewhere with no route to GitHub, such as a gh-aw sandbox.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const usage = `Usage: ghmock <command> [flags]

Runs gh binaries against a fake GitHub API and records every request.

Commands:
  run     Run gh against the mock
          ghmock run --gh PATH [--gh PATH ...] [--tty] [--stdin TEXT] [--logged-out] [--timeout DURATION] -- ARGS
  set     Change the response for a request
          ghmock set REQUEST [--status N] [--header 'Name: value'] [--body JSON | --body-file PATH]
                             [--at PATH --json VALUE] [--index N | --append]
  log     Show the requests from the last run
          ghmock log [--all] [--full] [--json]
  reset   Delete all responses and the request log

Requests are named "METHOD /path" for REST, such as "GET /repos/octo/hello",
and "graphql OperationName" for GraphQL, such as "graphql IssueByNumber".

State lives in $GHMOCK_DIR, or in a ghmock folder in the system temp directory.
ghmock exits non-zero only when ghmock itself fails. gh's exit code is in the report.

ghmock does not block network access. Run it somewhere with no route to GitHub.
`

// usageError marks errors caused by invalid arguments, which exit with status 2.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }

func (e usageError) Unwrap() error { return e.err }

func newUsageError(format string, args ...any) error {
	return usageError{fmt.Errorf(format, args...)}
}

func main() {
	os.Exit(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

func runMain(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	var err error
	switch args[0] {
	case "run":
		err = cmdRun(args[1:], stdout)
	case "set":
		err = cmdSet(args[1:], stdout)
	case "log":
		err = cmdLog(args[1:], stdout)
	case "reset":
		err = cmdReset(args[1:], stdout)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "ghmock: unknown command %q\n\n%s", args[0], usage)
		return 2
	}

	if err == nil {
		return 0
	}
	fmt.Fprintf(stderr, "ghmock %s: %v\n", args[0], err)
	if _, ok := errors.AsType[usageError](err); ok {
		return 2
	}
	return 1
}
