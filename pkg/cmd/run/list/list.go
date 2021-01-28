package list

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/run/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

const (
	defaultLimit = 10
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)

	ShowProgress bool
	PlainOutput  bool

	Limit int
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:    "list",
		Short:  "List recent workflow runs",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			terminal := opts.IO.IsStdoutTTY() && opts.IO.IsStdinTTY()
			opts.ShowProgress = terminal
			opts.PlainOutput = !terminal

			if opts.Limit < 1 {
				return &cmdutil.FlagError{Err: fmt.Errorf("invalid limit: %v", opts.Limit)}
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", defaultLimit, "Maximum number of runs to fetch")

	return cmd
}

func listRun(opts *ListOptions) error {
	if opts.ShowProgress {
		opts.IO.StartProgressIndicator()
	}
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("failed to determine base repo: %w", err)
	}

	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	runs, err := shared.GetRuns(client, baseRepo, opts.Limit)
	if err != nil {
		return fmt.Errorf("failed to get runs: %w", err)
	}

	tp := utils.NewTablePrinter(opts.IO)

	cs := opts.IO.ColorScheme()
	out := opts.IO.Out

	if opts.ShowProgress {
		opts.IO.StopProgressIndicator()
	}

	if len(runs) == 0 {
		if !opts.PlainOutput {
			fmt.Fprintln(out, "No runs found")
		}
		return nil
	}

	for _, run := range runs {
		if opts.PlainOutput {
			tp.AddField(string(run.Status), nil, nil)
			tp.AddField(string(run.Conclusion), nil, nil)
		} else {
			tp.AddField(shared.Symbol(cs, run.Status, run.Conclusion), nil, nil)
		}

		tp.AddField(run.CommitMsg(), nil, cs.Bold)

		tp.AddField(run.Name, nil, nil)
		tp.AddField(run.HeadBranch, nil, cs.Bold)
		tp.AddField(string(run.Event), nil, nil)

		if opts.PlainOutput {
			elapsed := run.UpdatedAt.Sub(run.CreatedAt)
			if elapsed < 0 {
				elapsed = 0
			}
			tp.AddField(elapsed.String(), nil, nil)
		}

		tp.AddField(fmt.Sprintf("%d", run.ID), nil, cs.Cyan)

		tp.EndRow()
	}

	err = tp.Render()
	if err != nil {
		return err
	}

	if !opts.PlainOutput {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "For details on a run, try: gh run view <run-id>")
	}

	return nil
}
