// Command ghmock runs gh binaries against a fake GitHub API.
//
// ghmock points HTTPS_PROXY at a server on localhost. When gh opens a tunnel to
// github.com, or to any other host, ghmock completes the TLS handshake itself,
// with a certificate from a throwaway authority that gh trusts through
// SSL_CERT_FILE. So gh behaves as it does against github.com, while every call
// lands on responses ghmock controls, and ghmock records each request. With
// --localhost, gh uses github.localhost instead, which it calls over plain
// HTTP, for gh builds that ignore SSL_CERT_FILE.
//
// Routing is all ghmock does: it does not block network access, so it must run
// somewhere with no route to GitHub, such as a gh-aw sandbox.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
)

const usage = `Usage: ghmock <command> [flags]

Runs gh binaries against a fake GitHub API and records every request.

Commands:
  run      Run gh against the mock
           ghmock run --gh PATH [--gh PATH ...] [--tty | --terminal] [--keys KEYS ...] [--stdin TEXT]
                      [--logged-out] [--localhost] [--timeout DURATION] -- ARGS
  set      Change the response for a request
           ghmock set REQUEST [--status N] [--header 'Name: value'] [--body JSON | --body-file PATH]
                              [--at PATH --json VALUE] [--index N | --append] [--off-schema]
  log      Show the requests from the last run
           ghmock log [--all] [--full] [--json]
  reset    Delete all responses, the request log and saved runs
  scaffold Add placeholder responses that pass the schema check
           ghmock scaffold REQUEST | --missing
  export   Write runs, requests and responses to a folder, to check results
           ghmock export DIR
  schema   Look up a GraphQL type or field, or a REST request, in GitHub's schemas
           ghmock schema Issue | Issue.title | 'GET /repos/octo/hello' [--status N] [--at PATH]
  schemas  Download GitHub's schemas again
           ghmock schemas update

gh talks to ghmock as if it were github.com, so run commands as a report wrote
them. ghmock answers HTTPS for every host gh connects to, with certificates
from a throwaway authority that gh trusts through SSL_CERT_FILE. Requests are
named "METHOD /path" for REST, such as "GET /repos/octo/hello", "graphql
OperationName" for GraphQL, such as "graphql IssueByNumber", and "METHOD
host/path" for other hosts, such as "GET uploads.github.com/upload".
GraphQL requests use operationName when supplied. Comparisons include REST
query strings and GraphQL query text and variables, for reads and writes.
The default log selects the latest completed run, even if it made no requests.

GraphQL placeholders are minimal: each list has one item, numbers are 1,
strings are the field's name and booleans are false, except has...Enabled
fields. So pageInfo.hasNextPage is false, and gh stops after one page.

When GitHub can't find what a query looks up, such as an organization whose
login belongs to a user, it answers null plus an errors entry, and gh often
depends on that error:
  ghmock set "graphql UserOrgOwner" --at data.organization --json null
  ghmock set "graphql UserOrgOwner" --at errors --json '[{"type":"NOT_FOUND","path":["organization"],"message":"Could not resolve to an Organization with the login of octo."}]'

ghmock set rejects a response that doesn't match GitHub's schema, unless you
pass --off-schema. ghmock run reports responses, GraphQL queries and variables
that don't match.
GitHub answers a query that doesn't match with an error, but ghmock still sends
the stored response, so read the report as well as gh's exit code.

ghmock run stops gh after --timeout, a minute by default. For a hang or a loop,
the report lists the requests gh made before it stopped.

With --tty, gh writes as it would to a 120-column color terminal, with
TERM=xterm-256color and COLORTERM=truecolor. The report shows each escape
character as \e, such as \e[33m for yellow and \e[0m for a reset.

To answer gh's prompts, give --keys for each prompt, such as
--keys '{enter}' --keys 'my-repo{enter}'. gh then runs in a real terminal, and
ghmock types each entry once the screen has been still for half a second. Keys
are {enter} {tab} {space} {backspace} {esc} {up} {down} {left} {right}
{ctrl-c} {ctrl-d}, and {{ types a brace. --terminal alone shows the first
prompt: ghmock stops gh when it waits for input with no keys left. The report
shows the screen before each --keys entry and at the end. stdout and stderr
share the screen, as they do for a user.

GraphQL responses are trimmed to the fields gh asks for, as GitHub does, so one
stored response can serve gh versions that ask for different fields. Off-schema
responses are sent as stored.
Trimming and scaffolding use the selected operation and typename aliases.

ghmock scaffold --missing adds a response for each request in the last run that
had none, and adds fields gh asked for that a stored GraphQL response lacks. It
never changes a value that's already there. GraphQL placeholders come from
gh's query, and REST bodies from GitHub's examples. Change what matters with
ghmock set, then run gh again.

--localhost makes gh use github.localhost over plain HTTP instead of
github.com, for gh builds that ignore SSL_CERT_FILE, such as macOS builds made
with older Go.

The schemas come from $GHMOCK_GRAPHQL_SCHEMA and $GHMOCK_REST_SCHEMA, or are
downloaded into ghmock's cache folder the first time they're needed. ghmock
schemas update downloads them again, to those paths when they're set.

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

// askedForHelp reports whether args include a help flag. Commands that take no
// flags use it, since only a flag set returns flag.ErrHelp on its own.
func askedForHelp(args []string) bool {
	return slices.ContainsFunc(args, func(a string) bool {
		return a == "-h" || a == "-help" || a == "--help"
	})
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
		err = cmdRun(args[1:], stdout, stderr)
	case "set":
		err = cmdSet(args[1:], stdout, stderr)
	case "log":
		err = cmdLog(args[1:], stdout)
	case "reset":
		err = cmdReset(args[1:], stdout)
	case "scaffold":
		err = cmdScaffold(args[1:], stdout, stderr)
	case "export":
		err = cmdExport(args[1:], stdout)
	case "schema":
		err = cmdSchema(args[1:], stdout, stderr)
	case "schemas":
		err = cmdSchemas(args[1:], stdout, stderr)
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
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "ghmock %s: %v\n", args[0], err)
	if _, ok := errors.AsType[usageError](err); ok {
		return 2
	}
	return 1
}
