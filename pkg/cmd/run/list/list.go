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

// TODO filters
// --state=(pending,pass,fail,etc)
// --active - pending
// --workflow - filter by workflow name

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
		// TODO better err handle
		return err
	}

	c, err := opts.HttpClient()
	if err != nil {
		// TODO better error handle
		return err
	}
	client := api.NewClientFromHTTP(c)

	runs, err := shared.GetRuns(client, baseRepo, opts.Limit)
	if err != nil {
		// TODO better error handle
		return err
	}

	tp := utils.NewTablePrinter(opts.IO)

	cs := opts.IO.ColorScheme()

	if opts.ShowProgress {
		opts.IO.StopProgressIndicator()
	}
	for _, run := range runs {
		//idStr := cs.Cyanf("%d", run.ID)
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
			tp.AddField(elapsed.String(), nil, nil)
		}

		tp.AddField(fmt.Sprintf("%d", run.ID), nil, cs.Cyan)

		tp.EndRow()
	}

	err = tp.Render()
	if err != nil {
		// TODO better error handle
		return err
	}

	fmt.Fprintln(opts.IO.Out)
	fmt.Fprintln(opts.IO.Out, "For details on a run, try: gh run view <run-id>")

	return nil
}
