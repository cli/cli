package rename

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type RenameOptions struct{
	HttpClient func() (*http.Client, error)
	IO *iostreams.IOStreams
	RepoName string
}

func MewCmcRename(f *cmdutil.Factory, runf func(*RenameOptions) error) *cobra.Command {
	opts:= &RenameOptions {
		IO: f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use: "rename <user/repo> <user/repo_change",
		Short: "Rename a repository",
		Long: "Rename a GitHub repository",
		Args: cmdutil.ExactArgs(2, "cannot rename: repository argument required"),
		RunE: func (cmd *cobra.Command, args []string) error {
			opts.RepoName = args[0]
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
	apiClient := api.NewClientFromHTTP(httpClient);

	var toRename ghrepo.Interface

	repoURL := opts.RepoName

	if !strings.Contains(repoURL, "/") {
		currentUser, err := api.CurrentLoginName(apiClient, ghinstance.Default())
		if err != nil {
			return err
		}
		repoURL = currentUser + "/" + repoURL
	}

	toRename, err = ghrepo.FromFullName(repoURL)
	if err != nil {
		return fmt.Errorf("argument error: %w", err)
	}

	fields := []string{"name", "owner", "id"}

	repo, err := 
}