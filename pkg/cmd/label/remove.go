package label

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type removeOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Name string
}

func newCmdRemove(f *cmdutil.Factory, runF func(*removeOptions) error) *cobra.Command {
	opts := removeOptions{
		Browser:    f.Browser,
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a label from a repository",
		Args:  cmdutil.ExactArgs(1, "cannot remove label: name argument required"),
		RunE: func(c *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.Name = args[0]

			if runF != nil {
				return runF(&opts)
			}
			return removeRun(&opts)
		},
	}

	return cmd
}

func removeRun(opts *removeOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	err = removeLabel(httpClient, baseRepo, opts.Name)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		successMsg := fmt.Sprintf("%s Label %q removed from %s\n", cs.SuccessIcon(), opts.Name, ghrepo.FullName(baseRepo))
		fmt.Fprint(opts.IO.Out, successMsg)
	}

	return nil
}

func removeLabel(client *http.Client, repo ghrepo.Interface, name string) error {
	apiClient := api.NewClientFromHTTP(client)
	path := fmt.Sprintf("repos/%s/%s/labels/%s", repo.RepoOwner(), repo.RepoName(), name)

	return apiClient.REST(repo.RepoHost(), "DELETE", path, nil, nil)
}
