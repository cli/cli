package checks

import (
	"fmt"
	"sort"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type browser interface {
	Browse(string) error
}

type ChecksOptions struct {
	IO      *iostreams.IOStreams
	Browser browser

	Finder shared.PRFinder

	SelectorArg string
	WebMode     bool
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

			return checksRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the web browser to show details about checks")

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

func addRow(tp utils.TablePrinter, isTerminal bool, cs *iostreams.ColorScheme, o check) {
	elapsed := ""
	zeroTime := time.Time{}

	if o.StartedAt != zeroTime && o.CompletedAt != zeroTime {
		e := o.CompletedAt.Sub(o.StartedAt)
		if e > 0 {
			elapsed = e.String()
		}
	}

	mark := "✓"
	markColor := cs.Green
	switch o.Bucket {
	case "fail":
		mark = "X"
		markColor = cs.Red
	case "pending":
		mark = "*"
		markColor = cs.Yellow
	case "skipping":
		mark = "-"
		markColor = cs.Gray
	}

	if isTerminal {
		tp.AddField(mark, nil, markColor)
		tp.AddField(o.Name, nil, nil)
		tp.AddField(elapsed, nil, nil)
		tp.AddField(o.Link, nil, nil)
	} else {
		tp.AddField(o.Name, nil, nil)
		tp.AddField(o.Bucket, nil, nil)
		if elapsed == "" {
			tp.AddField("0", nil, nil)
		} else {
			tp.AddField(elapsed, nil, nil)
		}
		tp.AddField(o.Link, nil, nil)
	}

	tp.EndRow()
}

func checksRun(opts *ChecksOptions) error {
	checks, meta, err := collect(opts)
	if err != nil {
		return fmt.Errorf("cannot collect checks information: %w", err)
	}

	fmt.Printf("%+v\n", checks)

	isTerminal := opts.IO.IsStdoutTTY()
	cs := opts.IO.ColorScheme()

	summary := ""
	if meta.Failed+meta.Passed+meta.Skipping+meta.Pending > 0 {
		if meta.Failed > 0 {
			summary = "Some checks were not successful"
		} else if meta.Pending > 0 {
			summary = "Some checks are still pending"
		} else {
			summary = "All checks were successful"
		}

		tallies := fmt.Sprintf("%d failing, %d successful, %d skipped, and %d pending checks",
			meta.Failed, meta.Passed, meta.Skipping, meta.Pending)

		summary = fmt.Sprintf("%s\n%s", cs.Bold(summary), tallies)
	}

	if isTerminal {
		fmt.Fprintln(opts.IO.Out, summary)
		fmt.Fprintln(opts.IO.Out)
	}

	tp := utils.NewTablePrinter(opts.IO)

	sort.Slice(checks, func(i, j int) bool {
		b0 := checks[i].Bucket
		n0 := checks[i].Name
		l0 := checks[i].Link
		b1 := checks[j].Bucket
		n1 := checks[j].Name
		l1 := checks[j].Link

		if b0 == b1 {
			if n0 == n1 {
				return l0 < l1
			}
			return n0 < n1
		}

		return (b0 == "fail") || (b0 == "pending" && b1 == "success")
	})

	for _, o := range checks {
		addRow(tp, isTerminal, cs, o)
	}

	err = tp.Render()
	if err != nil {
		return err
	}

	if meta.Failed+meta.Pending > 0 {
		return cmdutil.SilentError
	}

	return nil
}
