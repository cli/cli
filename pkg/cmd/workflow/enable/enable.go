package enable

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type EnableOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	Selector string
	Prompt   bool
}

func NewCmdEnable(f *cmdutil.Factory, runF func(*EnableOptions) error) *cobra.Command {
	opts := &EnableOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "enable [<workflow-id> | <workflow-name>]",
		Short: "Enable a workflow",
		Long:  "Enable a workflow, allowing it to be run and show up when listing workflows.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.Selector = args[0]
			} else if !opts.IO.CanPrompt() {
				return &cmdutil.FlagError{Err: errors.New("workflow ID or name required when not running interactively")}
			} else {
				opts.Prompt = true
			}

			if runF != nil {
				return runF(opts)
			}
			return runEnable(opts)
		},
	}

	return cmd
}

func runEnable(opts *EnableOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	states := []shared.WorkflowState{shared.DisabledManually}
	workflow, err := shared.ResolveWorkflow(
		opts.IO, client, repo, opts.Prompt, opts.Selector, states)
	if err != nil {
		var fae shared.FilteredAllError
		if errors.As(err, &fae) {
			return errors.New("there are no disabled workflows to enable")
		}
		return err
	}

	path := fmt.Sprintf("repos/%s/actions/workflows/%d/enable", ghrepo.FullName(repo), workflow.ID)
	err = client.REST(repo.RepoHost(), "PUT", path, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to enable workflow: %w", err)
	}

	if opts.IO.CanPrompt() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s Enabled %s\n", cs.SuccessIcon(), cs.Bold(workflow.Name))
	}

	return nil
}
