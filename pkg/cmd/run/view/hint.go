package view

import (
	"github.com/cli/cli/v2/internal/ghrepo"
	eventShared "github.com/cli/cli/v2/pkg/cmd/event/shared"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
)

// printEventListenHint points agents at `gh event listen`, because agents tend to poll `gh run view` when asked to
// wait for the checks of a pull request to finish. A completed run gets no hint, as there is nothing left to wait for.
func printEventListenHint(opts *ViewOptions, repo ghrepo.Interface, run *shared.Run, prNumber int) {
	if opts.InvokingAgent == "" || run.Status == shared.Completed {
		return
	}
	eventShared.PrintChecksListenHint(opts.IO.ErrOut, ghrepo.GenerateRepoURL(repo, "pull/%d", prNumber))
}
