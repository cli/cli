package list

import (
	"fmt"
	"net/http"
	"time"

	"github.com/MakeNowJust/heredoc"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	workflowShared "github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const (
	defaultLimit = 20
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Prompter   iprompter

	Exporter cmdutil.Exporter

	Limit            int
	WorkflowSelector string
	Branch           string
	Actor            string
	Status           string
	Event            string
	Created          string
	Commit           string
	All              bool

	now time.Time
}

type iprompter interface {
	Select(string, string, []string) (int, error)
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Prompter:   f.Prompter,
		now:        time.Now(),
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent workflow runs",
		Long: heredoc.Docf(`
			List recent workflow runs.

			Note that providing the %[1]sworkflow_name%[1]s to the %[1]s-w%[1]s flag will not fetch disabled workflows.
			Also pass the %[1]s-a%[1]s flag to fetch disabled workflow runs using the %[1]sworkflow_name%[1]s and the %[1]s-w%[1]s flag.
		`, "`"),
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

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
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "Filter runs by branch")
	cmd.Flags().StringVarP(&opts.Actor, "user", "u", "", "Filter runs by user who triggered the run")
	cmd.Flags().StringVarP(&opts.Event, "event", "e", "", "Filter runs by which `event` triggered the run")
	cmd.Flags().StringVarP(&opts.Created, "created", "", "", "Filter runs by the `date` it was created")
	cmd.Flags().StringVarP(&opts.Commit, "commit", "c", "", "Filter runs by the `SHA` of the commit")
	cmd.Flags().BoolVarP(&opts.All, "all", "a", false, "Include disabled workflows")
	cmdutil.StringEnumFlag(cmd, &opts.Status, "status", "s", "", shared.AllStatuses, "Filter runs by status")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.RunFields)

	_ = cmdutil.RegisterBranchCompletionFlags(f.GitClient, cmd, "branch")

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

	filters := &shared.FilterOptions{
		Branch:  opts.Branch,
		Actor:   opts.Actor,
		Status:  opts.Status,
		Event:   opts.Event,
		Created: opts.Created,
		Commit:  opts.Commit,
	}

	opts.IO.StartProgressIndicator()
	if opts.WorkflowSelector != "" {
		// initially the workflow state is limited to 'active'
		states := []workflowShared.WorkflowState{workflowShared.Active}
		if opts.All {
			// the all flag tells us to add the remaining workflow states
			// note: this will be incomplete if more workflow states are added to `workflowShared`
			states = append(states, workflowShared.DisabledManually, workflowShared.DisabledInactivity)
		}
		if workflow, err := workflowShared.ResolveWorkflow(opts.Prompter, opts.IO, client, baseRepo, false, opts.WorkflowSelector, states); err == nil {
			filters.WorkflowID = workflow.ID
			filters.WorkflowName = workflow.Name
		} else {
			return err
		}
	}
	runsResult, err := shared.GetRuns(client, baseRepo, filters, opts.Limit)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("failed to get runs: %w", err)
	}
	runs := runsResult.WorkflowRuns
	if len(runs) == 0 && opts.Exporter == nil {
		return cmdutil.NewNoResultsError("no runs found")
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, runs)
	}

	tp := tableprinter.New(opts.IO, tableprinter.WithHeader("STATUS", "TITLE", "WORKFLOW", "BRANCH", "EVENT", "ID", "ELAPSED", "AGE"))

	cs := opts.IO.ColorScheme()

	for _, run := range runs {
		if tp.IsTTY() {
			symbol, symbolColor := shared.Symbol(cs, run.Status, run.Conclusion)
			tp.AddField(symbol, tableprinter.WithColor(symbolColor))
		} else {
			tp.AddField(string(run.Status))
			tp.AddField(string(run.Conclusion))
		}
		tp.AddField(run.Title(), tableprinter.WithColor(cs.Bold))
		tp.AddField(run.WorkflowName())
		tp.AddField(run.HeadBranch, tableprinter.WithColor(cs.Bold))
		tp.AddField(string(run.Event))
		tp.AddField(fmt.Sprintf("%d", run.ID), tableprinter.WithColor(cs.Cyan))
		tp.AddField(run.Duration(opts.now).String())
		tp.AddTimeField(opts.now, run.StartedTime(), cs.Gray)
		tp.EndRow()
	}

	if err := tp.Render(); err != nil {
		return err
	}

	return nil
}
