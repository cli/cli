package api

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/cli/cli/v2/internal/ghrepo"
	eventShared "github.com/cli/cli/v2/pkg/cmd/event/shared"
)

var pullRequestPathRE = regexp.MustCompile(`^/?repos/([^/?#]+)/([^/?#]+)/pulls/(\d+)(?:[/?#].*)?$`)

// printEventListenHint points agents at `gh event listen` after a successful read of a pull request REST endpoint,
// because agents tend to poll such endpoints when asked to wait for a pull request to change.
func printEventListenHint(opts *ApiOptions, host, method, requestPath string) {
	if opts.InvokingAgent == "" || !strings.EqualFold(method, "GET") {
		return
	}
	if strings.Contains(requestPath, "://") {
		// An absolute URL decides the host on its own. Event subscriptions are only available on github.com, so only
		// its API URLs, such as the api_url of an event, can get the hint.
		u, err := url.Parse(requestPath)
		if err != nil || !strings.EqualFold(u.Host, "api.github.com") {
			return
		}
		host, requestPath = "github.com", u.Path
	}
	m := pullRequestPathRE.FindStringSubmatch(requestPath)
	if m == nil {
		return
	}
	repo := ghrepo.NewWithHost(m[1], m[2], host)
	eventShared.PrintListenHint(opts.IO.ErrOut, ghrepo.GenerateRepoURL(repo, "pull/%s", m[3]))
}
