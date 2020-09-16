package status

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/text"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type StatusOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)
	Branch     func() (string, error)

	HasRepoOverride bool
}

func NewCmdStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Remotes:    f.Remotes,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of relevant pull requests",
		Args:  cmdutil.NoArgsQuoteReminder,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.HasRepoOverride = cmd.Flags().Changed("repo")

			if runF != nil {
				return runF(opts)
			}
			return statusRun(opts)
		},
	}

	return cmd
}

func statusRun(opts *StatusOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	var currentBranch string
	var currentPRNumber int
	var currentPRHeadRef string

	if !opts.HasRepoOverride {
		currentBranch, err = opts.Branch()
		if err != nil && !errors.Is(err, git.ErrNotOnAnyBranch) {
			return fmt.Errorf("could not query for pull request for current branch: %w", err)
		}

		remotes, _ := opts.Remotes()
		currentPRNumber, currentPRHeadRef, err = prSelectorForCurrentBranch(baseRepo, currentBranch, remotes)
		if err != nil {
			return fmt.Errorf("could not query for pull request for current branch: %w", err)
		}
	}

	// the `@me` macro is available because the API lookup is ElasticSearch-based
	currentUser := "@me"
	prPayload, err := api.PullRequests(apiClient, baseRepo, currentPRNumber, currentPRHeadRef, currentUser)
	if err != nil {
		return err
	}

	err = opts.IO.StartPager()
	if err != nil {
		return err
	}
	defer opts.IO.StopPager()

	out := opts.IO.Out

	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "Relevant pull requests in %s\n", ghrepo.FullName(baseRepo))
	fmt.Fprintln(out, "")

	shared.PrintHeader(out, "Current branch")
	currentPR := prPayload.CurrentPR
	if currentPR != nil && currentPR.State != "OPEN" && prPayload.DefaultBranch == currentBranch {
		currentPR = nil
	}
	if currentPR != nil {
		printPrs(out, 1, *currentPR)
	} else if currentPRHeadRef == "" {
		shared.PrintMessage(out, "  There is no current branch")
	} else {
		shared.PrintMessage(out, fmt.Sprintf("  There is no pull request associated with %s", utils.Cyan("["+currentPRHeadRef+"]")))
	}
	fmt.Fprintln(out)

	shared.PrintHeader(out, "Created by you")
	if prPayload.ViewerCreated.TotalCount > 0 {
		printPrs(out, prPayload.ViewerCreated.TotalCount, prPayload.ViewerCreated.PullRequests...)
	} else {
		shared.PrintMessage(out, "  You have no open pull requests")
	}
	fmt.Fprintln(out)

	shared.PrintHeader(out, "Requesting a code review from you")
	if prPayload.ReviewRequested.TotalCount > 0 {
		printPrs(out, prPayload.ReviewRequested.TotalCount, prPayload.ReviewRequested.PullRequests...)
	} else {
		shared.PrintMessage(out, "  You have no pull requests to review")
	}
	fmt.Fprintln(out)

	return nil
}

func prSelectorForCurrentBranch(baseRepo ghrepo.Interface, prHeadRef string, rem context.Remotes) (prNumber int, selector string, err error) {
	selector = prHeadRef
	branchConfig := git.ReadBranchConfig(prHeadRef)

	// the branch is configured to merge a special PR head ref
	prHeadRE := regexp.MustCompile(`^refs/pull/(\d+)/head$`)
	if m := prHeadRE.FindStringSubmatch(branchConfig.MergeRef); m != nil {
		prNumber, _ = strconv.Atoi(m[1])
		return
	}

	var branchOwner string
	if branchConfig.RemoteURL != nil {
		// the branch merges from a remote specified by URL
		if r, err := ghrepo.FromURL(branchConfig.RemoteURL); err == nil {
			branchOwner = r.RepoOwner()
		}
	} else if branchConfig.RemoteName != "" {
		// the branch merges from a remote specified by name
		if r, err := rem.FindByName(branchConfig.RemoteName); err == nil {
			branchOwner = r.RepoOwner()
		}
	}

	if branchOwner != "" {
		if strings.HasPrefix(branchConfig.MergeRef, "refs/heads/") {
			selector = strings.TrimPrefix(branchConfig.MergeRef, "refs/heads/")
		}
		// prepend `OWNER:` if this branch is pushed to a fork
		if !strings.EqualFold(branchOwner, baseRepo.RepoOwner()) {
			selector = fmt.Sprintf("%s:%s", branchOwner, prHeadRef)
		}
	}

	return
}

func printPrs(w io.Writer, totalCount int, prs ...api.PullRequest) {
	for _, pr := range prs {
		prNumber := fmt.Sprintf("#%d", pr.Number)

		prStateColorFunc := utils.Green
		if pr.IsDraft {
			prStateColorFunc = utils.Gray
		} else if pr.State == "MERGED" {
			prStateColorFunc = utils.Magenta
		} else if pr.State == "CLOSED" {
			prStateColorFunc = utils.Red
		}

		fmt.Fprintf(w, "  %s  %s %s", prStateColorFunc(prNumber), text.Truncate(50, text.ReplaceExcessiveWhitespace(pr.Title)), utils.Cyan("["+pr.HeadLabel()+"]"))

		checks := pr.ChecksStatus()
		reviews := pr.ReviewStatus()

		if pr.State == "OPEN" {
			reviewStatus := reviews.ChangesRequested || reviews.Approved || reviews.ReviewRequired
			if checks.Total > 0 || reviewStatus {
				// show checks & reviews on their own line
				fmt.Fprintf(w, "\n  ")
			}

			if checks.Total > 0 {
				var summary string
				if checks.Failing > 0 {
					if checks.Failing == checks.Total {
						summary = utils.Red("× All checks failing")
					} else {
						summary = utils.Red(fmt.Sprintf("× %d/%d checks failing", checks.Failing, checks.Total))
					}
				} else if checks.Pending > 0 {
					summary = utils.Yellow("- Checks pending")
				} else if checks.Passing == checks.Total {
					summary = utils.Green("✓ Checks passing")
				}
				fmt.Fprint(w, summary)
			}

			if checks.Total > 0 && reviewStatus {
				// add padding between checks & reviews
				fmt.Fprint(w, " ")
			}

			if reviews.ChangesRequested {
				fmt.Fprint(w, utils.Red("+ Changes requested"))
			} else if reviews.ReviewRequired {
				fmt.Fprint(w, utils.Yellow("- Review required"))
			} else if reviews.Approved {
				fmt.Fprint(w, utils.Green("✓ Approved"))
			}
		} else {
			fmt.Fprintf(w, " - %s", shared.StateTitleWithColor(pr))
		}

		fmt.Fprint(w, "\n")
	}
	remaining := totalCount - len(prs)
	if remaining > 0 {
		fmt.Fprintf(w, utils.Gray("  And %d more\n"), remaining)
	}
}
