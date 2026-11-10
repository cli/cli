package view

import (
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	eventShared "github.com/cli/cli/v2/pkg/cmd/event/shared"
)

// printEventListenHint points agents at `gh event listen`, because agents tend to poll `gh pr view` when asked to wait
// for a pull request to change.
func printEventListenHint(opts *ViewOptions, baseRepo ghrepo.Interface, pr *api.PullRequest) {
	if opts.InvokingAgent == "" || pr.State == "MERGED" || pr.State == "CLOSED" {
		return
	}
	eventShared.PrintListenHint(opts.IO.ErrOut, pullRequestResource(baseRepo, pr))
}

func pullRequestResource(baseRepo ghrepo.Interface, pr *api.PullRequest) string {
	if pr.URL != "" {
		return pr.URL
	}
	if baseRepo != nil && pr.Number > 0 {
		return ghrepo.GenerateRepoURL(baseRepo, "pull/%d", pr.Number)
	}
	return ""
}
