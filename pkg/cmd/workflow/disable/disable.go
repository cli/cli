package disable

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DisableOptions struct {
	GitHubREST func(host string, opts ...githubrest.ClientOption) (*githubrest.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Prompter   iprompter

	Selector string
	Prompt   bool
}

type iprompter interface {
	Select(string, string, []string) (int, error)
}

func NewCmdDisable(f *cmdutil.Factory, runF func(*DisableOptions) error) *cobra.Command {
	opts := &DisableOptions{
		IO:         f.IOStreams,
		GitHubREST: f.GitHubREST,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "disable [<workflow-id> | <workflow-name>]",
		Short: "Disable a workflow",
		Long:  "Disable a workflow, preventing it from running or showing up when listing workflows.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.Selector = args[0]
			} else if !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("workflow ID or name required when not running interactively")
			} else {
				opts.Prompt = true
			}

			if runF != nil {
				return runF(opts)
			}
			return runDisable(cmd.Context(), opts)
		},
	}

	return cmd
}

func runDisable(ctx context.Context, opts *DisableOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	client, err := opts.GitHubREST(repo.RepoHost())
	if err != nil {
		return fmt.Errorf("could not build client: %w", err)
	}

	states := []shared.WorkflowState{shared.Active}
	workflow, err := shared.ResolveWorkflow(
		ctx, opts.Prompter, opts.IO, client, repo, opts.Prompt, opts.Selector, states)
	if err != nil {
		var fae shared.FilteredAllError
		if errors.As(err, &fae) {
			return errors.New("there are no enabled workflows to disable")
		}
		return err
	}

	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "workflows", strconv.FormatInt(workflow.ID, 10), "disable")
	if err != nil {
		return err
	}
	req, err := client.NewRequest(ctx, http.MethodPut, path.String(), nil)
	if err != nil {
		return err
	}
	if _, err := client.Do(req, nil); err != nil {
		return fmt.Errorf("failed to disable workflow: %w", err)
	}

	if opts.IO.CanPrompt() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s Disabled %s\n", cs.SuccessIconWithColor(cs.Red), cs.Bold(workflow.Name))
	}

	return nil
}
