package create

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

type createOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	Branch     func() (string, error)
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Ref         string
	Environment string
	Status      string
}

type deployment struct {
	ID int `json:"id"`
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*createOptions) error) *cobra.Command {
	opts := createOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new deployment",
		Long: heredoc.Docf(`
			Create a new deployment.

			This will dispatch a %[1]sdeployment%[1]s event that external services can listen for and act on.
		`, "`"),
		Example: heredoc.Doc(`
			# Create a deployment on the current branch
			$ gh deployment create

			# Create a deployment at a specified ref
			$ gh deployment create --ref main

			# Create a deployment for a specific environment
			$ gh deployment create --environment test

			# Create a deployment with an initial status
			$ gh deployment create --status in_progress
		`),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			// Validate the status flag
			if opts.Status != "" {
				switch opts.Status {
				case "queued", "in_progress", "inactive", "in_review", "approved", "rejected", "pending", "error", "failure", "success":
				default:
					return fmt.Errorf("invalid status: %s", opts.Status)
				}
			}

			if runF != nil {
				return runF(&opts)
			}
			return createRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Ref, "ref", "r", "", "The `branch`, `tag` or `SHA` to deploy (default [current branch])")
	cmd.Flags().StringVarP(&opts.Environment, "environment", "e", "", "The `environment` that the deployment is for")
	cmd.Flags().StringVarP(&opts.Status, "status", "s", "", "The initial `status` of the deployment")

	_ = cmdutil.RegisterBranchCompletionFlags(f.GitClient, cmd, "ref")

	return cmd
}

func createRun(opts *createOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}
	client := api.NewClientFromHTTP(httpClient)

	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	ref := opts.Ref

	if ref == "" {
		ref, err = opts.Branch()
		if err != nil {
			return fmt.Errorf("could not determine the current branch: %w", err)
		}
	}

	var deployment deployment
	err = createDeployment(client, opts, repo, ref, opts.Environment, &deployment)
	if err != nil {
		return err
	}

	if opts.Status != "" {
		err = createDeploymentStatus(client, opts, repo, deployment)
		if err != nil {
			return err
		}
	}

	if opts.IO.IsStdoutTTY() {
		out := opts.IO.Out
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(out, "%s Created deployment for %s in %s\n",
			cs.SuccessIcon(), cs.Bold(ref), ghrepo.FullName(repo))
	} else {
		fmt.Fprintf(opts.IO.Out, "%d\n", deployment.ID)
	}

	return nil
}

func createDeployment(client *api.Client, opts *createOptions, repo ghrepo.Interface, ref string, environment string, result *deployment) error {
	path := fmt.Sprintf("repos/%s/%s/deployments",
		repo.RepoOwner(), repo.RepoName())

	request := struct {
		Ref         string `json:"ref,omitempty"`
		Environment string `json:"environment,omitempty"`
	}{
		Ref:         ref,
		Environment: environment,
	}

	requestByte, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to serialize deployment inputs: %w", err)
	}

	body := bytes.NewReader(requestByte)

	opts.IO.StartProgressIndicator()
	err = client.REST(repo.RepoHost(), "POST", path, body, &result)
	opts.IO.StopProgressIndicator()

	if err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	return nil
}

func createDeploymentStatus(client *api.Client, opts *createOptions, repo ghrepo.Interface, deployment deployment) error {
	path := fmt.Sprintf("repos/%s/%s/deployments/%d/statuses",
		repo.RepoOwner(), repo.RepoName(), deployment.ID)

	request := struct {
		State string `json:"state"`
	}{
		State: opts.Status,
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
