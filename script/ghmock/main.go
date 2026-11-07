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
                      [--logged-out | --config-dir DIR] [--localhost] [--in DIR] [--timeout DURATION] -- ARGS
  repo     Create an empty fake remote repository
           ghmock repo [HOST/]OWNER/REPO [--branch NAME]
  git      Prepare a Git fixture or inspect a retained build's workspace
           ghmock git [--in DIR] [--run ID|latest] [--build N | --before]
                      [--stdin TEXT] [--timeout DURATION] -- GIT_ARGS
  write    Write a file inside the prepared Git workspace, creating its directories
           ghmock write PATH --text TEXT
  set      Change the response for a request
           ghmock set REQUEST [--status N] [--header 'Name: value'] [--body JSON | --body-file PATH]
                              [--raw-file PATH | --zip-dir DIR]
                              [--at PATH --json VALUE] [--index N | --append] [--off-schema]
  log      Show the requests from the last run
           ghmock log [--all] [--full] [--json]
  reset    Delete all responses, logs, saved runs and retained Git/config state
  scaffold Add placeholder responses that pass the schema check
           ghmock scaffold REQUEST | --missing
  export   Write runs, requests and responses to a folder or portable archive
           ghmock export [--archive] [PATH]
  schema   Look up a GraphQL type or field, or a REST request, in GitHub's schemas
           ghmock schema Issue | Issue.title | 'GET /repos/example.invalid/hello' [--status N] [--at PATH]
  schemas  Download GitHub's schemas again
           ghmock schemas update

gh talks to ghmock as if it were github.com, so run commands as a report wrote
them. ghmock answers HTTPS for every host gh connects to, with certificates
from a throwaway authority that gh trusts through SSL_CERT_FILE. Requests are
named "METHOD /path" for REST, such as "GET /repos/example.invalid/hello", "graphql
OperationName" for GraphQL, such as "graphql IssueByNumber", and "METHOD
host/path" for other hosts, such as "GET uploads.github.com/upload".
GraphQL requests use operationName when supplied. Comparisons include REST
query strings and GraphQL query text and variables, for reads and writes.
The default log selects the latest completed run, even if it made no requests.

Use example.invalid/hello for synthetic repositories. GitHub account names
cannot contain periods, so these URLs cannot identify a real repository.
Keep github.com as the host.

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

For non-JSON responses, --raw-file sends a file byte-for-byte. --zip-dir builds
a ZIP response from regular files in a directory, including hidden files; it
rejects symlinks instead of following them. This supports logs and artifact
downloads without needing a shell ZIP utility. --zip-dir sets Content-Type to
application/zip unless you override it. Set Content-Type for other raw formats.
Raw bodies are never compacted or trimmed. JSON logs and exports retain their
exact bytes as base64 in raw_body/response_raw_body fields; log --full shows
their size and SHA-256 instead of printing binary data to the terminal.

ghmock run stops gh after --timeout, a minute by default. For a hang or a loop,
the report lists the requests gh made before it stopped.

With --tty, gh writes as it would to a 120-column color terminal, with
TERM=xterm-256color and COLORTERM=truecolor. The report shows each escape
character as \e, such as \e[33m for yellow and \e[0m for a reset.

To answer gh's prompts, give --keys for each prompt, such as
--keys '{enter}' --keys 'my-repo{enter}'. gh then runs in a real terminal, and
ghmock types each entry once the screen has been still for half a second. Keys
are {enter} {tab} {space} {backspace} {esc} {up} {down} {left} {right}
{ctrl-c} {ctrl-d}, and {{ types a brace. Terminal runs obey --timeout; quiet
output alone does not mean a prompt is waiting. Use a short --timeout to
inspect an unanswered prompt. The report retains raw terminal output and
the screen before each --keys entry and at the end. stdout and stderr share
the terminal, as they do for a user. Terminal input requires Unix; other
modes are available on Windows.

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

--config-dir copies a configuration fixture once, then gives each gh build its
own writable copy as GH_CONFIG_DIR. Supply hosts.yml and/or config.yml as needed;
no default account or config values are added to the fixture. The input directory
is never modified. An empty fixture models a logged-out state. Files are copied
verbatim, including malformed configuration for testing; contained symlinks and
special files are rejected. With --localhost, name fixture hosts github.localhost.
The before/after copies are retained under the state folder's config/runs and
included in exports. Use synthetic credentials only. This supplies files, not
an isolated OS keyring; run authentication scenarios inside a sandbox without
access to the operator's credential store.

export uses GHMOCK_EXPORT_PATH when PATH is omitted. --archive writes tar.gz,
including hidden Git metadata and relative worktree links, without overwriting
an existing file. Extract into an empty folder; portable linked worktrees need
Git 2.48 or newer. Explicit directory exports keep their existing behavior.

Git uses real repositories, not canned command output:
  ghmock repo example.invalid/hello
  ghmock git -- clone https://github.com/example.invalid/hello.git main
  ghmock write main/example.txt --text 'first version'
  ghmock git --in main -- add example.txt
  ghmock git --in main -- commit -m 'Add example'
  ghmock git --in main -- push origin main
  ghmock git --in main -- worktree add ../feature -b feature
  ghmock run --gh ./bin/gh --in feature -- repo sync
  ghmock git --run latest --build 1 --in main -- status --porcelain=v1

--in is a directory in the fixture workspace, not a repository name. Each run
snapshots the complete fixture, including remotes, files, index and linked
worktrees, then gives each gh build its own copy. --run selects that retained
copy for inspection; --before selects the run's original snapshot. Without
--run, git and write change only the fixture for future runs. reset deletes
both the fixture and retained copies. export includes them with repaired paths.

HTTPS uses git http-backend. SSH URLs use a local upload/receive-pack helper,
not real SSH, so this does not test keys or host authentication. github.localhost
is an alias for github.com fixtures. Git uses a fake author and fixed dates.
API responses are still configured with set/scaffold; API mutations do not
automatically create Git repositories or refs. Keep their URLs and SHAs aligned.

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
	case "repo":
		err = cmdRepo(args[1:], stdout)
	case "git":
		err = cmdGit(args[1:], stdout, stderr)
	case "write":
		err = cmdWrite(args[1:], stdout)
	case "_git-ssh":
		err = cmdGitSSH(args[1:], os.Stdin, stdout, stderr)
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
