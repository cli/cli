package rename

import (
	"fmt"
	"net/http"

	// "strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"

	// "github.com/cli/cli/v2/internal/ghinstance"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type RenameOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	RepoName   []string
}

func NewCmdRename(f *cmdutil.Factory, runf func(*RenameOptions) error) *cobra.Command {
	opts := &RenameOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use:   "rename <user/repo_name> <user/new_repo_name>",
		Short: "Rename a repository",
		Long:  "Rename a GitHub repository",
		Args:  cmdutil.ExactArgs(2, "cannot rename: repository argument required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RepoName = append(opts.RepoName, args[0], args[1])
			if runf != nil {
				return runf(opts)
			}
			return renameRun(opts)
		},
	}
	return cmd
}

func renameRun(opts *RenameOptions) error {
	oldRepoName := opts.RepoName[0]

	// cs := opts.IO.ColorScheme()
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	username, err := api.CurrentLoginName(apiClient, ghinstance.Default())
	if err != nil {
		return err
	}

	repo, err := ghrepo.FromFullName(oldRepoName)
	if err != nil {
		return fmt.Errorf("argument error: %w", err)
	}

	fields := []string{"name", "owner", "id"}
	repoDetails, err := api.FetchRepository(apiClient, repo, fields)
	if err != nil {
		return err
	}

	if username != repoDetails.Owner.Login {
		return fmt.Errorf("you do not own this repository")
	}

	err = renameRepo(apiClient, repo.RepoHost(), repoDetails)
	if err != nil {
		return err
	}

	return nil
}

func renameRepo(apiClient *api.Client, hostname string, repoDetails *api.Repository) error {
	path := fmt.Sprintf("/repos/%s/%s", repoDetails.Owner.Login, repoDetails.Name)
	fmt.Println(path);

	return nil
}
