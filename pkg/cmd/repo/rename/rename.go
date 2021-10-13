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
	HttpClient      func() (*http.Client, error)
	IO              *iostreams.IOStreams
	Config          func() (config.Config, error)
	oldRepoSelector string
	newRepoSelector string
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

		Use:   "rename <repository> <new-name>",
		Short: "Rename a repository",
		Long:  "Rename a GitHub repository",
		Args:  cmdutil.ExactArgs(2, "cannot rename: repository argument required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.oldRepoSelector = args[0]
			opts.newRepoSelector = args[1]
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

	oldRepoURL := opts.oldRepoSelector
	if !strings.Contains(oldRepoURL, "/") {
		currentUser, err := api.CurrentLoginName(apiClient, ghinstance.Default())
		if err != nil {
			return err
		}
		oldRepoURL = currentUser + "/" + oldRepoURL
	}
	newRepoName := opts.newRepoSelector

	repo, err := ghrepo.FromFullName(oldRepoURL)
	if err != nil {
		return fmt.Errorf("argument error: %w", err)
	}

	input := renameRepo{
		Owner:      repo.RepoOwner(),
		Repository: repo.RepoName(),
		Name:       newRepoName,
	}

	err = runRename(apiClient, repo.RepoHost(), input)
	if err != nil {
		return fmt.Errorf("API called failed: %s, please check your parameters", err)
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "%s Renamed repository %s\n", cs.SuccessIcon(), repo.RepoOwner()+"/"+newRepoName)
	}

	return nil
}

func runRename(apiClient *api.Client, hostname string, input renameRepo) error {
	path := fmt.Sprintf("repos/%s/%s", input.Owner, input.Repository)

	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	if err := enc.Encode(input); err != nil {
		return err
	}

	return apiClient.REST(hostname, "PATCH", path, body, nil)
}
