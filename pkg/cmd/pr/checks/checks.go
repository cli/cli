package checks

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

const defaultInterval int = 30

type browser interface {
	Browse(string) error
}

type ChecksOptions struct {
	IO      *iostreams.IOStreams
	Browser browser

	Finder shared.PRFinder

	SelectorArg string
	WebMode     bool
	Interval    int
	Watch       bool
}

func NewCmdChecks(f *cmdutil.Factory, runF func(*ChecksOptions) error) *cobra.Command {
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
				return cmdutil.FlagErrorf("argument required when using the --repo flag")
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}

			if opts.WebMode {
				return checksRunWebMode(opts)
			}

			if opts.Watch {
				return checksRunWatchMode(opts)
			}

			return checksRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the web browser to show details about checks")
	cmd.Flags().BoolVarP(&opts.Watch, "watch", "", false, "Watch checks and wait until are finished running")
	cmd.Flags().IntVarP(&opts.Interval, "interval", "i", defaultInterval, "The number of seconds between each reprint When using --watch")

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

func checksRunWatchMode(opts *ChecksOptions) error {
	duration, err := time.ParseDuration(fmt.Sprintf("%ds", opts.Interval))
	if err != nil {
		return fmt.Errorf("could not parse interval: %w", err)
	}

	checks, meta, err := collect(opts)
	if err != nil {
		return fmt.Errorf("cannot collect checks information: %w", err)
	}

	isTerminal := opts.IO.IsStdoutTTY()
	cs := opts.IO.ColorScheme()

	err = printTable(isTerminal, cs, meta, checks, opts)
	if err != nil {
		return fmt.Errorf("cannot print table: %w", err)
	}

	// Intercept SIGINT to trap CTRL+C and exit appropriately
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		if meta.Failed+meta.Pending > 0 {
			// NOTE: given is not possible to return an error from here
			// exit with error as checksRun() does
			os.Exit(1)
		}
		os.Exit(0)
	}()

	for meta.Pending > 0 {
		time.Sleep(duration)

		if err := opts.IO.EnableVirtualTerminalProcessing(); err == nil {
			// ANSI escape code to clean terminal window
			// https://stackoverflow.com/a/22892171
			fmt.Print(opts.IO.Out, "\033[H\033[2J")
		}

		checks, meta, err = collect(opts)
		if err != nil {
			return fmt.Errorf("cannot collect checks information: %w", err)
		}

		err = printTable(isTerminal, cs, meta, checks, opts)
		if err != nil {
			return fmt.Errorf("cannot print table: %w", err)
		}
	}

	return nil
}

func checksRun(opts *ChecksOptions) error {
	checks, meta, err := collect(opts)
	if err != nil {
		return fmt.Errorf("cannot collect checks information: %w", err)
	}

	isTerminal := opts.IO.IsStdoutTTY()
	cs := opts.IO.ColorScheme()

	err = printTable(isTerminal, cs, meta, checks, opts)
	if err != nil {
		return fmt.Errorf("cannot print table: %w", err)
	}

	if meta.Failed+meta.Pending > 0 {
		return cmdutil.SilentError
	}

	return nil
}
