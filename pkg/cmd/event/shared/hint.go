package shared

import (
	"fmt"
	"io"
	"net/url"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmd/event/client"
)

// PrintListenHint suggests `gh event listen` for the given pull request URL, because agents tend to poll when asked to
// wait for a pull request to change. It prints nothing for hosts the Events API does not support.
//
// The suggested command exits on the first event by default, which suits agent harnesses: they resume an agent when
// a background command exits, not when it prints.
func PrintListenHint(w io.Writer, pullRequestURL string) {
	printListenHint(w, pullRequestURL, listenHint{
		purpose:  "if you need to wait for this pull request to be reviewed, merged, or closed before continuing",
		events:   "com.github.pull_request.review.submitted.v1,com.github.pull_request.state.changed.v1",
		readNote: "Events carry no details; read the pull request to see what changed.",
		snapshot: "the pull request",
	})
}

// PrintChecksListenHint suggests `gh event listen` for the check suites of the given pull request URL, because agents
// tend to poll workflow runs when asked to wait for a pull request's checks to finish. It prints nothing for hosts the
// Events API does not support.
func PrintChecksListenHint(w io.Writer, pullRequestURL string) {
	printListenHint(w, pullRequestURL, listenHint{
		purpose: "if you need to wait for the checks of this pull request to finish before continuing",
		events:  "com.github.pull_request.check_suite.status.changed.v1",
		readNote: fmt.Sprintf("Each check suite, such as a workflow run, sends its own event with its conclusion "+
			"when it completes; after each event, read the checks with `gh pr checks %s`, and if any are still "+
			"pending, run the listener again.", pullRequestURL),
		snapshot: "the checks",
	})
}

type listenHint struct {
	// purpose completes "hint: ..." and says what the agent may be waiting for.
	purpose string
	// events is the comma-separated event types to listen to.
	events string
	// readNote says what an event carries and what to read after it.
	readNote string
	// snapshot names what to read once the listener is running.
	snapshot string
}

func printListenHint(w io.Writer, pullRequestURL string, h listenHint) {
	u, err := url.Parse(pullRequestURL)
	if pullRequestURL == "" || err != nil || !client.SupportedHost(u.Hostname()) {
		return
	}
	fmt.Fprint(w, heredoc.Docf(`
		hint: %[2]s, use this instead of polling:
		    gh event listen --resource %[3]s --event %[4]s
		  - It prints the first event as a JSON line on stdout and exits; run it again to wait for the next one.
		  - Add %[1]s--timeout%[1]s to stop waiting after a while, for example %[1]s--timeout 30m%[1]s; it then exits with status 8.
		  - %[5]s
		  - It misses changes made before it starts, so read %[6]s once it is running.
		  - See %[1]sgh event listen --help%[1]s for other event types.
		  - Event subscriptions are in preview and may not be enabled for your account.
	`, "`", h.purpose, pullRequestURL, h.events, h.readNote, h.snapshot))
}
