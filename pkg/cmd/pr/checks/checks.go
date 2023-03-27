package checks

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const defaultInterval time.Duration = 10 * time.Second

type ChecksOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Browser    browser.Browser

	Finder shared.PRFinder

	SelectorArg string
	WebMode     bool
	Interval    time.Duration
	Watch       bool
	FailFast    bool
	Required    bool
}

func NewCmdChecks(f *cmdutil.Factory, runF func(*ChecksOptions) error) *cobra.Command {
	var interval int
	opts := &ChecksOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		Browser:    f.Browser,
		Interval:   defaultInterval,
	}

	cmd := &cobra.Command{
		Use:   "checks [<number> | <url> | <branch>]",
		Short: "Show CI status for a single pull request",
		Long: heredoc.Doc(`
			Show CI status for a single pull request.

			Without an argument, the pull request that belongs to the current branch
			is selected.
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return cmdutil.FlagErrorf("argument required when using the `--repo` flag")
			}

			if opts.FailFast && !opts.Watch {
				return cmdutil.FlagErrorf("cannot use `--fail-fast` flag without `--watch` flag")
			}

			intervalChanged := cmd.Flags().Changed("interval")
			if !opts.Watch && intervalChanged {
				return cmdutil.FlagErrorf("cannot use `--interval` flag without `--watch` flag")
			}

			if intervalChanged {
				var err error
				opts.Interval, err = time.ParseDuration(fmt.Sprintf("%ds", interval))
				if err != nil {
					return cmdutil.FlagErrorf("could not parse `--interval` flag: %w", err)
				}
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}

			return checksRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the web browser to show details about checks")
	cmd.Flags().BoolVarP(&opts.Watch, "watch", "", false, "Watch checks until they finish")
	cmd.Flags().BoolVarP(&opts.FailFast, "fail-fast", "", false, "Exit watch mode on first check failure")
	cmd.Flags().IntVarP(&interval, "interval", "i", 10, "Refresh interval in seconds when using `--watch` flag")
	cmd.Flags().BoolVar(&opts.Required, "required", false, "Only show checks that are required")

	return cmd
}

func checksRunWebMode(opts *ChecksOptions) error {
	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"number"},
	}
	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	isTerminal := opts.IO.IsStdoutTTY()
	openURL := ghrepo.GenerateRepoURL(baseRepo, "pull/%d/checks", pr.Number)

	if isTerminal {
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
	}

	return opts.Browser.Browse(openURL)
}

func checksRun(opts *ChecksOptions) error {
	if opts.WebMode {
		return checksRunWebMode(opts)
	}

	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"number", "headRefName"},
	}

	var pr *api.PullRequest
	pr, repo, findErr := opts.Finder.Find(findOptions)
	if findErr != nil {
		return findErr
	}

	client, clientErr := opts.HttpClient()
	if clientErr != nil {
		return clientErr
	}

	var checks []check
	var counts checkCounts
	var err error

	checks, counts, err = populateStatusChecks(client, repo, pr, opts.Required)
	if err != nil {
		return err
	}

	if opts.Watch {
		opts.IO.StartAlternateScreenBuffer()
	} else {
		// Only start pager in non-watch mode
		if err := opts.IO.StartPager(); err == nil {
			defer opts.IO.StopPager()
		} else {
			fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
		}
	}

	// Do not return err until we can StopAlternateScreenBuffer()
	for {
		if counts.Pending != 0 && opts.Watch {
			opts.IO.RefreshScreen()
			cs := opts.IO.ColorScheme()
			fmt.Fprintln(opts.IO.Out, cs.Boldf("Refreshing checks status every %v seconds. Press Ctrl+C to quit.\n", opts.Interval.Seconds()))
		}

		printSummary(opts.IO, counts)
		err = printTable(opts.IO, checks)
		if err != nil {
			break
		}

		if counts.Pending == 0 || !opts.Watch {
			break
		}

		if opts.FailFast && counts.Failed > 0 {
			break
		}

		time.Sleep(opts.Interval)

		checks, counts, err = populateStatusChecks(client, repo, pr, opts.Required)
		if err != nil {
			break
		}
	}

	opts.IO.StopAlternateScreenBuffer()
	if err != nil {
		return err
	}

	if opts.Watch {
		// Print final summary to original screen buffer
		printSummary(opts.IO, counts)
		err = printTable(opts.IO, checks)
		if err != nil {
			return err
		}
	}

	if counts.Failed+counts.Pending > 0 {
		return cmdutil.SilentError
	}

	return nil
}

func populateStatusChecks(client *http.Client, repo ghrepo.Interface, pr *api.PullRequest, requiredChecks bool) ([]check, checkCounts, error) {
	apiClient := api.NewClientFromHTTP(client)

	type response struct {
		Node *api.PullRequest
	}

	query := fmt.Sprintf(`
	query PullRequestStatusChecks($id: ID!, $endCursor: String) {
		node(id: $id) {
			...on PullRequest {
				%s
			}
		}
	}`, api.RequiredStatusCheckRollupGraphQL("$id", "$endCursor"))

	variables := map[string]interface{}{
		"id": pr.ID,
	}

	statusCheckRollup := api.CheckContexts{}

	for {
		var resp response
		err := apiClient.GraphQL(repo.RepoHost(), query, variables, &resp)
		if err != nil {
			return nil, checkCounts{}, err
		}

		if len(resp.Node.StatusCheckRollup.Nodes) == 0 {
			return nil, checkCounts{}, errors.New("no commit found on the pull request")
		}

		result := resp.Node.StatusCheckRollup.Nodes[0].Commit.StatusCheckRollup.Contexts
		statusCheckRollup.Nodes = append(
			statusCheckRollup.Nodes,
			result.Nodes...,
		)

		if !result.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = result.PageInfo.EndCursor
	}

	if len(statusCheckRollup.Nodes) == 0 {
		return nil, checkCounts{}, fmt.Errorf("no checks reported on the '%s' branch", pr.HeadRefName)
	}

	checks, counts := aggregateChecks(statusCheckRollup.Nodes, requiredChecks)
	if len(checks) == 0 && requiredChecks {
		return checks, counts, fmt.Errorf("no required checks reported on the '%s' branch", pr.HeadRefName)
	}
	return checks, counts, nil
}
