package cancel

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CancelOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	RunID string
}

func NewCmdCancel(f *cmdutil.Factory, runF func(*CancelOptions) error) *cobra.Command {
	opts := &CancelOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "cancel <run-id>",
		Short: "Cancel a workflow run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.RunID = args[0]

			if runF != nil {
				return runF(opts)
			}

			return runCancel(opts)
		},
	}

	return cmd
}

func runCancel(opts *CancelOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}
	client := api.NewClientFromHTTP(httpClient)

	cs := opts.IO.ColorScheme()

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("failed to determine base repo: %w", err)
	}

	err = cancelWorkflowRun(client, repo, opts.RunID)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) {
			if httpErr.StatusCode == http.StatusConflict {
				err = fmt.Errorf("Cannot cancel a workflow run that is completed")
			} else if httpErr.StatusCode == http.StatusNotFound {
				err = fmt.Errorf("Could not find any workflow run with ID %s", opts.RunID)
			}
		}

		return err
	}

	fmt.Fprintf(opts.IO.Out, "%s You have successfully requested the workflow to be canceled.", cs.SuccessIcon())

	return nil
}

func cancelWorkflowRun(client *api.Client, repo ghrepo.Interface, runID string) error {
	path := fmt.Sprintf("repos/%s/actions/runs/%s/cancel", ghrepo.FullName(repo), runID)

	err := client.REST(repo.RepoHost(), "POST", path, nil, nil)
	if err != nil {
		return err
	}

	return nil
}
