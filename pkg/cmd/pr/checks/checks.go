package checks

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ChecksOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Branch     func() (string, error)
	Remotes    func() (context.Remotes, error)

	SelectorArg string
}

func NewCmdChecks(f *cmdutil.Factory, runF func(*ChecksOptions) error) *cobra.Command {
	opts := &ChecksOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Branch:     f.Branch,
		Remotes:    f.Remotes,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "checks",
		Short: "Show CI status for a single pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return &cmdutil.FlagError{Err: errors.New("argument required when using the --repo flag")}
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

	return cmd
}

func checksRun(opts *ChecksOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	pr, _, err := shared.PRFromArgs(apiClient, opts.BaseRepo, opts.Branch, opts.Remotes, opts.SelectorArg)
	if err != nil {
		return err
	}

	if len(pr.Commits.Nodes) == 0 {
		return nil
	}

	rollup := pr.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes
	if len(rollup) == 0 {
		return nil
	}

	passing := 0
	failing := 0
	pending := 0

	type output struct {
		mark    string
		bucket  string
		name    string
		elapsed string
		link    string
	}

	outputs := []output{}

	for _, c := range pr.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes {
		mark := ""
		bucket := ""
		state := c.State
		if state == "" {
			if c.Status == "COMPLETED" {
				state = c.Conclusion
			} else {
				state = c.Status
			}
		}
		switch state {
		case "SUCCESS", "NEUTRAL", "SKIPPED":
			mark = utils.GreenCheck()
			passing++
			bucket = "pass"
		case "ERROR", "FAILURE", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED":
			mark = utils.RedX()
			failing++
			bucket = "fail"
		case "EXPECTED", "REQUESTED", "QUEUED", "PENDING", "IN_PROGRESS", "STALE":
			mark = utils.YellowDash()
			pending++
			bucket = "pending"
		default:
			panic(fmt.Errorf("unsupported status: %q", state))
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

		outputs = append(outputs, output{mark, bucket, name, elapsed, link})
	}

	sort.Slice(outputs, func(i, j int) bool {
		if outputs[i].bucket == outputs[j].bucket {
			return outputs[i].name < outputs[j].name
		} else {
			if outputs[i].bucket == "fail" {
				return true
			} else if outputs[i].bucket == "pending" && outputs[j].bucket == "success" {
				return true
			}
		}

		return false
	})

	tp := utils.NewTablePrinter(opts.IO)

	for _, o := range outputs {
		if opts.IO.IsStdoutTTY() {
			tp.AddField(o.mark, nil, nil)
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

	summary := ""
	if failing+passing+pending > 0 {
		if failing > 0 {
			summary = "Some checks were not successful"
		} else if pending > 0 {
			summary = "Some checks are still pending"
		} else {
			summary = "All checks were successful"
		}

		tallies := fmt.Sprintf(
			"%d failing, %d successful, and %d pending checks",
			failing, passing, pending)

		summary = fmt.Sprintf("%s\n%s", utils.Bold(summary), tallies)
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintln(opts.IO.Out, summary)
		fmt.Fprintln(opts.IO.Out)
	}

	return tp.Render()
}
