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
	u, err := url.Parse(pullRequestURL)
	if pullRequestURL == "" || err != nil || !client.SupportedHost(u.Hostname()) {
		return
	}
	fmt.Fprint(w, heredoc.Docf(`
		hint: if you need to wait for this pull request to be reviewed, merged, or closed before continuing, use this instead of polling:
		    gh event listen --resource %[2]s --event com.github.pull_request.review.submitted.v1,com.github.pull_request.state.changed.v1
		  - It prints the first event as a JSON line on stdout and exits; run it again to wait for the next one.
		  - Add %[1]s--timeout%[1]s to stop waiting after a while, for example %[1]s--timeout 30m%[1]s; it then exits with status 8.
		  - Events carry no details; read the pull request to see what changed.
		  - It misses changes made before it starts, so read the pull request once it is running.
		  - See %[1]sgh event listen --help%[1]s for other event types.
		  - Event subscriptions are in preview and may not be enabled for your account.
	`, "`", pullRequestURL))
}
