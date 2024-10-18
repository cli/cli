package delete

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type deleteOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Deployment string
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*deleteOptions) error) *cobra.Command {
	opts := deleteOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "delete <deployment>",
		Short: "Delete a deployment",
		Long: heredoc.Docf(`
			Delete a deployment.

			If the repository only has one deployment, you can delete the
			deployment regardless of its status. If the repository has more
			than one deployment, you can only delete inactive deployments.
		`, "`"),
		Example: heredoc.Doc(`
			# Delete a deployment in the current repository
			$ gh deployment delete 1234

			# Delete a deployment in a specific repository
			$ gh deployment delete --repo owner/repo 1234
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			opts.Deployment = args[0]

			if runF != nil {
				return runF(&opts)
			}
			return deleteRun(&opts)
		},
	}

	return cmd
}

func deleteRun(opts *deleteOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}
	client := api.NewClientFromHTTP(httpClient)

	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("repos/%s/%s/deployments/%s",
		repo.RepoOwner(), repo.RepoName(), opts.Deployment)

	opts.IO.StartProgressIndicator()
	err = client.REST(repo.RepoHost(), "DELETE", path, nil, nil)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("could not delete deployment: %w", err)
	}

	if opts.IO.IsStdoutTTY() {
		out := opts.IO.Out
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(out, "%s Deleted deployment %s from %s\n",
			cs.SuccessIcon(), cs.Bold(opts.Deployment), ghrepo.FullName(repo))
	}

	return nil
}
