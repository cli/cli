package update

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type updateOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Deployment     string
	Status         string
	LogURL         string
	Description    string
	Environment    string
	EnvironmentURL string
	AutoInactive   bool
}

func NewCmdUpdate(f *cmdutil.Factory, runF func(*updateOptions) error) *cobra.Command {
	opts := updateOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "update [flags] <deployment>",
		Short: "Update a deployment",
		Long: heredoc.Docf(`
			Update a deployment.

			To update a deployment, you must specify a %[1]sstatus%[1]s and can include details such as a %[1]slog-url%[1]s.

			This command will update the deployment by creating a new deployment status.
		`, "`"),
		Example: heredoc.Doc(`
			# Update a deployment to in_progress
			$ gh deployment update --status in_progress 1234

			# Update a deployment to add a log_url
			$ gh deployment update --log-url https://example.com 1234
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			opts.Deployment = args[0]

			if opts.Status == "" {
				return fmt.Errorf("status flag is required")
			}

			switch opts.Status {
			case "queued", "in_progress", "inactive", "in_review", "approved", "rejected", "pending", "error", "failure", "success":
			default:
				return fmt.Errorf("invalid status: %s", opts.Status)
			}

			if len(opts.Description) > 140 {
				return fmt.Errorf("description must be less than 140 characters")
			}

			if runF != nil {
				return runF(&opts)
			}
			return updateRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Status, "status", "s", "", "The `status` of the deployment")
	cmd.Flags().StringVarP(&opts.LogURL, "log-url", "l", "", "The full URL of the deployment output")
	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "A short description of the status")
	cmd.Flags().StringVarP(&opts.Environment, "environment", "e", "", "Name for the target deployment environment")
	cmd.Flags().StringVarP(&opts.EnvironmentURL, "environment-url", "u", "", "The URL for accessing your environment")
	cmd.Flags().BoolVar(&opts.AutoInactive, "auto-inactive", true, "Add a new inactive status to all prior non-transient, non-production environment deployments with the same repository and environment")

	return cmd
}

func updateRun(opts *updateOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}
	client := api.NewClientFromHTTP(httpClient)

	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	err = createDeploymentStatus(client, opts, repo)
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		out := opts.IO.Out
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(out, "%s Created deployment status in %s\n",
			cs.SuccessIcon(), ghrepo.FullName(repo))
	}

	return nil
}

func createDeploymentStatus(client *api.Client, opts *updateOptions, repo ghrepo.Interface) error {
	path := fmt.Sprintf("repos/%s/%s/deployments/%s/statuses",
		repo.RepoOwner(), repo.RepoName(), opts.Deployment)

	request := struct {
		State          string `json:"state"`
		LogURL         string `json:"log_url,omitempty"`
		Description    string `json:"description,omitempty"`
		Environment    string `json:"environment,omitempty"`
		EnvironmentURL string `json:"environment_url,omitempty"`
		AutoInactive   bool   `json:"auto_inactive"`
	}{
		State:          opts.Status,
		LogURL:         opts.LogURL,
		Description:    opts.Description,
		Environment:    opts.Environment,
		EnvironmentURL: opts.EnvironmentURL,
		AutoInactive:   opts.AutoInactive,
	}

	requestByte, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to serialize deployment status inputs: %w", err)
	}

	body := bytes.NewReader(requestByte)

	opts.IO.StartProgressIndicator()
	err = client.REST(repo.RepoHost(), "POST", path, body, nil)
	opts.IO.StopProgressIndicator()

	if err != nil {
		return fmt.Errorf("failed to create deployment status: %w", err)
	}

	return nil
}
