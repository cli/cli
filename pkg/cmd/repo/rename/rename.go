package rename

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type RenameOptions struct {
	HttpClient  func() (*http.Client, error)
	IO          *iostreams.IOStreams
	Config      func() (config.Config, error)
	oldRepoName string
	newRepoName string
}

type renameRepo struct {
	Owner      string
	Repository string
	Name       string `json:"name,omitempty"`
}

func NewCmdRename(f *cmdutil.Factory, runf func(*RenameOptions) error) *cobra.Command {
	opts := &RenameOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use:   "rename <user/old_repo_name> <new_repo_name>",
		Short: "Rename a repository",
		Long:  "Rename a GitHub repository",
		Args:  cmdutil.ExactArgs(2, "cannot rename: repository argument required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.oldRepoName = args[0]
			opts.newRepoName = args[1]
			if runf != nil {
				return runf(opts)
			}
			return renameRun(opts)
		},
	}
	return cmd
}

func renameRun(opts *RenameOptions) error {
	cs := opts.IO.ColorScheme()
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	oldRepoName := opts.oldRepoName
	var currentUser string
	if !strings.Contains(oldRepoName, "/") {
		currentUser, err = api.CurrentLoginName(apiClient, ghinstance.Default())
		if err != nil {
			return err
		}
		oldRepoName = currentUser + "/" + oldRepoName
	}
	newRepoName := opts.newRepoName

	repo, err := ghrepo.FromFullName(oldRepoName)
	if err != nil {
		return fmt.Errorf("argument error: %w", err)
	}

	fields := []string{"name", "owner", "id"}
	repoDetails, err := api.FetchRepository(apiClient, repo, fields)
	if err != nil {
		return err
	}

	if currentUser != repoDetails.Owner.Login {
		return fmt.Errorf("%s you do not own this repository", cs.FailureIcon())
	}

	input := renameRepo{
		Owner:      repo.RepoOwner(),
		Repository: repo.RepoName(),
		Name:       newRepoName,
	}

	err = runRename(apiClient, repo.RepoHost(), repoDetails, input)
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "%s Renamed repository %s\n", cs.SuccessIcon(), currentUser+"/"+newRepoName)
	}

	return nil
}

func runRename(apiClient *api.Client, hostname string, repoDetails *api.Repository, input renameRepo) error {
	path := fmt.Sprintf("repos/%s/%s", repoDetails.Owner.Login, repoDetails.Name)

	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	if err := enc.Encode(input); err != nil {
		return err
	}

	err := apiClient.REST(hostname, "PATCH", path, body, nil)
	if err != nil {
		return err
	}
	return nil
}
