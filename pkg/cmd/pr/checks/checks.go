package checks

import (
	"fmt"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

const defaultInterval int = 10

type browser interface {
	Browse(string) error
}

type ChecksOptions struct {
	IO      *iostreams.IOStreams
	Browser browser

	Finder shared.PRFinder

	SelectorArg string
	WebMode     bool
	Interval    time.Duration
	Watch       bool
}

func NewCmdChecks(f *cmdutil.Factory, runF func(*ChecksOptions) error) *cobra.Command {
	var interval int
	opts := &ChecksOptions{
		IO:      f.IOStreams,
		Browser: f.Browser,
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

			if !opts.Watch && cmd.Flags().Changed("interval") {
				return cmdutil.FlagErrorf("cannot use `--interval` flag without `--watch` flag")
			}

			var err error
			opts.Interval, err = time.ParseDuration(fmt.Sprintf("%ds", interval))
			if err != nil {
				return cmdutil.FlagErrorf("could not parse `--interval` flag: %w", err)
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
	cmd.Flags().IntVarP(&interval, "interval", "i", defaultInterval, "Refresh interval in seconds when using `--watch` flag")

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
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
	}

	return opts.Browser.Browse(openURL)
}

func checksRun(opts *ChecksOptions) error {
	if opts.WebMode {
		return checksRunWebMode(opts)
	}

	var checks []check
	var counts checkCounts

	for {
		findOptions := shared.FindOptions{
			Selector: opts.SelectorArg,
			Fields:   []string{"number", "baseRefName", "statusCheckRollup"},
		}
		pr, _, err := opts.Finder.Find(findOptions)
		if err != nil {
			return err
		}

		checks, counts, err = aggregateChecks(pr)
		if err != nil {
			return err
		}

		printSummary(opts.IO, counts)
		err = printTable(opts.IO, checks)
		if err != nil {
			return err
		}

		if counts.Pending == 0 || !opts.Watch {
			break
		}

		time.Sleep(opts.Interval)

		if err := opts.IO.EnableVirtualTerminalProcessing(); err == nil {
			// ANSI escape code to clean terminal window
			// https://stackoverflow.com/a/22892171
			fmt.Print(opts.IO.Out, "\033[H\033[2J")
		}
	}

	if counts.Failed+counts.Pending > 0 {
		return cmdutil.SilentError
	}

	return nil
}
