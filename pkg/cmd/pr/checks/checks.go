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
	Exporter    cmdutil.Exporter
}

type output struct {
	mark      string
	bucket    string
	name      string
	elapsed   string
	link      string
	markColor func(string) string
}

type checksSummary struct {
	Passing  int    `json:"passing"`
	Failing  int    `json:"failing"`
	Skipping int    `json:"skipping"`
	Pending  int    `json:"pending"`
	Summary  string `json:"summary"`
	Tallies  string `json:"tallies"`
}

// generateSummary generates the summary and tallies strings for a pr check.
// if cs is not nil, then a Bold style will be applied to the Summary.
func (s *checksSummary) generateSummary(cs *iostreams.ColorScheme) string {
	s.Summary = ""
	if s.Failing+s.Passing+s.Skipping+s.Pending > 0 {

		if s.Failing > 0 {
			s.Summary = "Some checks were not successful"
		} else if s.Pending > 0 {
			s.Summary = "Some checks are still pending"
		} else {
			s.Summary = "All checks were successful"
		}

		s.Tallies = fmt.Sprintf("%d failing, %d successful, %d skipped, and %d pending checks",
			s.Failing, s.Passing, s.Skipping, s.Pending)

		if cs != nil {
			s.Summary = fmt.Sprintf("%s\n%s", cs.Bold(s.Summary), s.Tallies)
		} else {
			s.Summary = fmt.Sprintf("%s\n%s", s.Summary, s.Tallies)
		}
	}

	return s.Summary
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

			return checksRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the web browser to show details about checks")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, []string{"bucket", "elapsed", "link", "mark", "markColor", "name"})

	return cmd
}

func checksRun(opts *ChecksOptions) error {
	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"number", "baseRefName", "statusCheckRollup"},
	}
	if opts.WebMode {
		findOptions.Fields = []string{"number"}
	}
	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	isTerminal := opts.IO.IsStdoutTTY()

	if opts.WebMode {
		openURL := ghrepo.GenerateRepoURL(baseRepo, "pull/%d/checks", pr.Number)
		if isTerminal {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return opts.Browser.Browse(openURL)
	}

	if len(pr.StatusCheckRollup.Nodes) == 0 {
		return fmt.Errorf("no commit found on the pull request")
	}

	rollup := pr.StatusCheckRollup.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes
	if len(rollup) == 0 {
		return fmt.Errorf("no checks reported on the '%s' branch", pr.BaseRefName)
	}

	s := &checksSummary{}

	cs := opts.IO.ColorScheme()

	outputs := []output{}

	for _, c := range pr.StatusCheckRollup.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes {
		mark := "✓"
		bucket := "pass"
		state := c.State
		markColor := cs.Green
		if state == "" {
			if c.Status == "COMPLETED" {
				state = c.Conclusion
			} else {
				state = c.Status
			}
		}
		switch state {
		case "SUCCESS":
			s.Passing++
		case "SKIPPED", "NEUTRAL":
			mark = "-"
			markColor = cs.Gray
			s.Skipping++
			bucket = "skipping"
		case "ERROR", "FAILURE", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED":
			mark = "X"
			markColor = cs.Red
			s.Failing++
			bucket = "fail"
		default: // "EXPECTED", "REQUESTED", "WAITING", "QUEUED", "PENDING", "IN_PROGRESS", "STALE"
			mark = "*"
			markColor = cs.Yellow
			s.Pending++
			bucket = "pending"
		}

		elapsed := ""
		zeroTime := time.Time{}

		if c.StartedAt != zeroTime && c.CompletedAt != zeroTime {
			e := c.CompletedAt.Sub(c.StartedAt)
			if e > 0 {
				elapsed = e.String()
			}
		}

		link := c.DetailsURL
		if link == "" {
			link = c.TargetURL
		}

		name := c.Name
		if name == "" {
			name = c.Context
		}

		outputs = append(outputs, output{mark, bucket, name, elapsed, link, markColor})
	}

	sort.Slice(outputs, func(i, j int) bool {
		b0 := outputs[i].bucket
		n0 := outputs[i].name
		l0 := outputs[i].link
		b1 := outputs[j].bucket
		n1 := outputs[j].name
		l1 := outputs[j].link

		if b0 == b1 {
			if n0 == n1 {
				return l0 < l1
			}
			return n0 < n1
		}

		return (b0 == "fail") || (b0 == "pending" && b1 == "success")
	})

	if opts.Exporter == nil {
		s.generateSummary(cs)
		if err = printTable(opts, outputs, pr.Number, s); err != nil {
			return err
		}
	} else {
		s.generateSummary(nil)
		if err = printJSON(opts, outputs, pr.Number, s); err != nil {
			return err
		}
	}

	if s.Failing+s.Pending > 0 {
		return cmdutil.SilentError
	}

	return nil
}

func printJSON(opts *ChecksOptions, outputs []output, prNumber int, s *checksSummary) error {
	jsonData := map[string]interface{}{}

	for _, o := range outputs {
		jsonData[o.name] = map[string]interface{}{
			"number":    prNumber,
			"mark":      o.mark,
			"bucket":    o.bucket,
			"name":      o.name,
			"elapsed":   o.elapsed,
			"link":      o.link,
			"markColor": o.markColor(o.mark),
		}
	}
	jsonData["passing"] = s.Passing
	jsonData["failing"] = s.Failing
	jsonData["skipping"] = s.Skipping
	jsonData["pending"] = s.Pending
	jsonData["summary"] = s.Summary
	jsonData["tallies"] = s.Tallies

	return opts.Exporter.Write(opts.IO, jsonData)
}

func printTable(opts *ChecksOptions, outputs []output, prNumber int, s *checksSummary) error {
	tp := utils.NewTablePrinter(opts.IO)
	isTerminal := opts.IO.IsStdoutTTY()

	for _, o := range outputs {
		if isTerminal {
			tp.AddField(o.mark, nil, o.markColor)
			tp.AddField(o.name, nil, nil)
			tp.AddField(o.elapsed, nil, nil)
			tp.AddField(o.link, nil, nil)
		} else {
			tp.AddField(o.name, nil, nil)
			tp.AddField(o.bucket, nil, nil)
			if o.elapsed == "" {
				tp.AddField("0", nil, nil)
			} else {
				tp.AddField(o.elapsed, nil, nil)
			}
			tp.AddField(o.link, nil, nil)
		}

		tp.EndRow()
	}

	if isTerminal {
		fmt.Fprintln(opts.IO.Out, s.Summary)
		fmt.Fprintln(opts.IO.Out)
	}

	return tp.Render()
}
