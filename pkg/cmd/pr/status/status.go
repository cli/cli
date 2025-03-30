package status

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type StatusOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (ghContext.Remotes, error)
	Branch     func() (string, error)

	HasRepoOverride bool
	Exporter        cmdutil.Exporter
	ConflictStatus  bool

	Detector fd.Detector
}

func NewCmdStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Config:     f.Config,
		Remotes:    f.Remotes,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of relevant pull requests",
		Long: heredoc.Docf(`
			Show status of relevant pull requests.

			The status shows a summary of pull requests that includes information such as
			pull request number, title, CI checks, reviews, etc.

			To see more details of CI checks, run %[1]sgh pr checks%[1]s.
		`, "`"),
		Args: cmdutil.NoArgsQuoteReminder,
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

	cmd.Flags().BoolVarP(&opts.ConflictStatus, "conflict-status", "c", false, "Display the merge conflict status of each pull request")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, api.PullRequestFields)

	return cmd
}

func statusRun(opts *StatusOptions) error {
	ctx := context.Background()
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRefRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	var currentBranchName string
	var currentPRNumber int
	var currentHeadRefBranchName string

	if !opts.HasRepoOverride {
		// We must be in a repo without the override
		currentBranchName, err = opts.Branch()
		if err != nil && !errors.Is(err, git.ErrNotOnAnyBranch) {
			return fmt.Errorf("could not query for pull request for current branch: %w", err)
		}

		branchConfig, err := opts.GitClient.ReadBranchConfig(ctx, currentBranchName)
		if err != nil {
			return err
		}
		// Determine if the branch is configured to merge to a special PR ref
		prHeadRE := regexp.MustCompile(`^refs/pull/(\d+)/head$`)
		if m := prHeadRE.FindStringSubmatch(branchConfig.MergeRef); m != nil {
			currentPRNumber, _ = strconv.Atoi(m[1])
		}

		if currentPRNumber == 0 {
			remotes, err := opts.Remotes()
			if err != nil {
				return err
			}
			// Suppressing these errors as we have other means of computing the PullRequestRefs when these fail.
			parsedPushRevision, _ := opts.GitClient.ParsePushRevision(ctx, currentBranchName)

			remotePushDefault, err := opts.GitClient.RemotePushDefault(ctx)
			if err != nil {
				return err
			}

			pushDefault, err := opts.GitClient.PushDefault(ctx)
			if err != nil {
				return err
			}

			prRefs, err := shared.ParsePRRefs(currentBranchName, branchConfig, parsedPushRevision, pushDefault, remotePushDefault, baseRefRepo, remotes)
			if err != nil {
				return err
			}
			currentHeadRefBranchName = prRefs.BranchName
		}

		if err != nil {
			return fmt.Errorf("could not query for pull request for current branch: %w", err)
		}
	}

	options := requestOptions{
		Username:       "@me",
		CurrentPR:      currentPRNumber,
		HeadRef:        currentHeadRefBranchName,
		ConflictStatus: opts.ConflictStatus,
	}
	if opts.Exporter != nil {
		options.Fields = opts.Exporter.Fields()
	}

	if opts.Detector == nil {
		cachedClient := api.NewCachedHTTPClient(httpClient, time.Hour*24)
		opts.Detector = fd.NewDetector(cachedClient, baseRefRepo.RepoHost())
	}
	prFeatures, err := opts.Detector.PullRequestFeatures()
	if err != nil {
		return err
	}
	options.CheckRunAndStatusContextCountsSupported = prFeatures.CheckRunAndStatusContextCounts

	prPayload, err := pullRequestStatus(httpClient, baseRefRepo, options)
	if err != nil {
		return err
	}

	err = opts.IO.StartPager()
	if err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if opts.Exporter != nil {
		data := map[string]interface{}{
			"currentBranch": nil,
			"createdBy":     prPayload.ViewerCreated.PullRequests,
			"needsReview":   prPayload.ReviewRequested.PullRequests,
		}
		if prPayload.CurrentPR != nil {
			data["currentBranch"] = prPayload.CurrentPR
		}
		return opts.Exporter.Write(opts.IO, data)
	}

	out := opts.IO.Out
	cs := opts.IO.ColorScheme()

	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "Relevant pull requests in %s\n", ghrepo.FullName(baseRefRepo))
	fmt.Fprintln(out, "")

	if !opts.HasRepoOverride {
		shared.PrintHeader(opts.IO, "Current branch")
		currentPR := prPayload.CurrentPR
		if currentPR != nil && currentPR.State != "OPEN" && prPayload.DefaultBranch == currentBranchName {
			currentPR = nil
		}
		if currentPR != nil {
			printPrs(opts.IO, 1, *currentPR)
		} else if currentHeadRefBranchName == "" {
			shared.PrintMessage(opts.IO, "  There is no current branch")
		} else {
			shared.PrintMessage(opts.IO, fmt.Sprintf("  There is no pull request associated with %s", cs.Cyan("["+currentHeadRefBranchName+"]")))
		}
		fmt.Fprintln(out)
	}

	shared.PrintHeader(opts.IO, "Created by you")
	if prPayload.ViewerCreated.TotalCount > 0 {
		printPrs(opts.IO, prPayload.ViewerCreated.TotalCount, prPayload.ViewerCreated.PullRequests...)
	} else {
		shared.PrintMessage(opts.IO, "  You have no open pull requests")
	}
	fmt.Fprintln(out)

	shared.PrintHeader(opts.IO, "Requesting a code review from you")
	if prPayload.ReviewRequested.TotalCount > 0 {
		printPrs(opts.IO, prPayload.ReviewRequested.TotalCount, prPayload.ReviewRequested.PullRequests...)
	} else {
		shared.PrintMessage(opts.IO, "  You have no pull requests to review")
	}
	fmt.Fprintln(out)

	return nil
}

func totalApprovals(pr *api.PullRequest) int {
	approvals := 0
	for _, review := range pr.LatestReviews.Nodes {
		if review.State == "APPROVED" {
			approvals++
		}
	}
	return approvals
}

func printPrs(io *iostreams.IOStreams, totalCount int, prs ...api.PullRequest) {
	w := io.Out
	cs := io.ColorScheme()

	for _, pr := range prs {
		prNumber := fmt.Sprintf("#%d", pr.Number)

		prStateColorFunc := cs.ColorFromString(shared.ColorForPRState(pr))

		fmt.Fprintf(w, "  %s  %s %s", prStateColorFunc(prNumber), text.Truncate(50, text.RemoveExcessiveWhitespace(pr.Title)), cs.Cyan("["+pr.HeadLabel()+"]"))

		checks := pr.ChecksStatus()
		reviews := pr.ReviewStatus()

		if pr.State == "OPEN" {
			reviewStatus := reviews.ChangesRequested || reviews.Approved || reviews.ReviewRequired
			if checks.Total > 0 || reviewStatus {
				// show checks & reviews on their own line
				fmt.Fprintf(w, "\n  ")
			}

			if checks.Total > 0 {
				summary := shared.PrCheckStatusSummaryWithColor(cs, checks)
				fmt.Fprint(w, summary)
			}

			if checks.Total > 0 && reviewStatus {
				// add padding between checks & reviews
				fmt.Fprint(w, " ")
			}

			if reviews.ChangesRequested {
				fmt.Fprint(w, cs.Red("+ Changes requested"))
			} else if reviews.ReviewRequired {
				fmt.Fprint(w, cs.Yellow("- Review required"))
			} else if reviews.Approved {
				numRequiredApprovals := pr.BaseRef.BranchProtectionRule.RequiredApprovingReviewCount
				gotApprovals := totalApprovals(&pr)
				s := fmt.Sprintf("%d", gotApprovals)
				if numRequiredApprovals > 0 {
					s = fmt.Sprintf("%d/%d", gotApprovals, numRequiredApprovals)
				}
				fmt.Fprint(w, cs.Green(fmt.Sprintf("✓ %s Approved", s)))
			}

			if pr.Mergeable == api.PullRequestMergeableMergeable {
				// prefer "No merge conflicts" to "Mergeable" as there is more to mergeability
				// than the git status. Missing or failing required checks prevent merging
				// even though a PR is technically mergeable, which is often a source of confusion.
				fmt.Fprintf(w, " %s", cs.Green("✓ No merge conflicts"))
			} else if pr.Mergeable == api.PullRequestMergeableConflicting {
				fmt.Fprintf(w, " %s", cs.Red("× Merge conflicts"))
			} else if pr.Mergeable == api.PullRequestMergeableUnknown {
				fmt.Fprintf(w, " %s", cs.Yellow("! Merge conflict status unknown"))
			}

			if pr.BaseRef.BranchProtectionRule.RequiresStrictStatusChecks {
				switch pr.MergeStateStatus {
				case "BEHIND":
					fmt.Fprintf(w, " %s", cs.Yellow("- Not up to date"))
				case "UNKNOWN", "DIRTY":
					// do not print anything
				default:
					fmt.Fprintf(w, " %s", cs.Green("✓ Up to date"))
				}
			}

			if pr.AutoMergeRequest != nil {
				fmt.Fprintf(w, " %s", cs.Green("✓ Auto-merge enabled"))
			}

		} else {
			fmt.Fprintf(w, " - %s", shared.StateTitleWithColor(cs, pr))
		}

		fmt.Fprint(w, "\n")
	}
	remaining := totalCount - len(prs)
	if remaining > 0 {
		fmt.Fprintf(w, cs.Gray("  And %d more\n"), remaining)
	}
}
