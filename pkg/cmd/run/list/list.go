package list

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	workflowShared "github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

const (
	defaultLimit = 20
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)

	PlainOutput bool
	Exporter    cmdutil.Exporter

	Limit            int
	WorkflowSelector string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent workflow runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			terminal := opts.IO.IsStdoutTTY() && opts.IO.IsStdinTTY()
			opts.PlainOutput = !terminal

			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.Limit)
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", defaultLimit, "Maximum number of runs to fetch")
	cmd.Flags().StringVarP(&opts.WorkflowSelector, "workflow", "w", "", "Filter runs by workflow")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.RunFields)

	return cmd
}

func listRun(opts *ListOptions) error {
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("failed to determine base repo: %w", err)
	}

	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	var runs []shared.Run
	var workflow *workflowShared.Workflow

	opts.IO.StartProgressIndicator()
	if opts.WorkflowSelector != "" {
		states := []workflowShared.WorkflowState{workflowShared.Active}
		workflow, err = workflowShared.ResolveWorkflow(
			opts.IO, client, baseRepo, false, opts.WorkflowSelector, states)
		if err == nil {
			runs, err = shared.GetRunsByWorkflow(client, baseRepo, opts.Limit, workflow.ID)
		}
	} else {
		runs, err = shared.GetRuns(client, baseRepo, opts.Limit)
	}
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("failed to get runs: %w", err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, runs)
	}

	tp := utils.NewTablePrinter(opts.IO)

	cs := opts.IO.ColorScheme()

	if len(runs) == 0 {
		if !opts.PlainOutput {
			fmt.Fprintln(opts.IO.ErrOut, "No runs found")
		}
		return nil
	}

	out := opts.IO.Out

	if !opts.PlainOutput {
		tp.AddField("STATUS", nil, nil)
		tp.AddField("NAME", nil, nil)
		tp.AddField("WORKFLOW", nil, nil)
		tp.AddField("BRANCH", nil, nil)
		tp.AddField("EVENT", nil, nil)
		tp.AddField("ID", nil, nil)
		tp.AddField("ELAPSED", nil, nil)
		tp.AddField("AGE", nil, nil)
		tp.EndRow()
	}

	for _, run := range runs {
		if opts.PlainOutput {
			tp.AddField(string(run.Status), nil, nil)
			tp.AddField(string(run.Conclusion), nil, nil)
		} else {
			symbol, symbolColor := shared.Symbol(cs, run.Status, run.Conclusion)
			tp.AddField(symbol, nil, symbolColor)
		}

		tp.AddField(run.CommitMsg(), nil, cs.Bold)

		tp.AddField(run.Name, nil, nil)
		tp.AddField(run.HeadBranch, nil, cs.Bold)
		tp.AddField(string(run.Event), nil, nil)
		tp.AddField(fmt.Sprintf("%d", run.ID), nil, cs.Cyan)

		elapsed := run.UpdatedAt.Sub(run.CreatedAt)
		if elapsed < 0 {
			elapsed = 0
		}
		tp.AddField(elapsed.String(), nil, nil)
		tp.AddField(utils.FuzzyAgoAbbr(time.Now(), run.CreatedAt), nil, nil)
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
