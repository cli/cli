package checks

import (
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

const defaultInterval time.Duration = 10 * time.Second

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
		IO:       f.IOStreams,
		Browser:  f.Browser,
		Interval: defaultInterval,
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
	cmd.Flags().IntVarP(&interval, "interval", "i", 10, "Refresh interval in seconds when using `--watch` flag")

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

	if opts.Watch {
		if err := opts.IO.EnableVirtualTerminalProcessing(); err != nil {
			return err
		}
	} else {
		// Only start pager in non-watch mode
		if err := opts.IO.StartPager(); err == nil {
			defer opts.IO.StopPager()
		} else {
			fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
		}
	}

	var checks []check
	var counts checkCounts

	for {
		findOptions := shared.FindOptions{
			Selector: opts.SelectorArg,
			Fields:   []string{"number", "headRefName", "statusCheckRollup"},
		}
		pr, _, err := opts.Finder.Find(findOptions)
		if err != nil {
			return err
		}

		checks, counts, err = aggregateChecks(pr)
		if err != nil {
			return err
		}

		if counts.Pending != 0 && opts.Watch {
			refreshScreen(opts.IO.Out)
			cs := opts.IO.ColorScheme()
			fmt.Fprintln(opts.IO.Out, cs.Boldf("Refreshing checks status every %v seconds. Press Ctrl+C to quit.\n", opts.Interval.Seconds()))
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
	}

	if counts.Failed+counts.Pending > 0 {
		return cmdutil.SilentError
	}

	return nil
}

func refreshScreen(w io.Writer) {
	if runtime.GOOS == "windows" {
		// Just clear whole screen; I wasn't able to get the nicer cursor movement thing working
		fmt.Fprintf(w, "\x1b[2J")
	} else {
		// Move cursor to 0,0
		fmt.Fprint(w, "\x1b[0;0H")
		// Clear from cursor to bottom of screen
		fmt.Fprint(w, "\x1b[J")
	}
}
