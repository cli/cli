package rename

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/spf13/cobra"
)

type RenameOptions struct {
	HttpClient      func() (*http.Client, error)
	IO              *iostreams.IOStreams
	Config          func() (config.Config, error)
	BaseRepo        func() (ghrepo.Interface, error)
	oldRepoSelector string
	newRepoSelector string
	flagRepo        bool
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
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "rename [-R] [<repository>] [<new-name>]",
		Short: "Rename a repository",
		Long: `Rename a GitHub repository
		With no argument, the repository for the current directory is renamed using a prompt
		With one argument, the repository of the current directory is renamed using the argument
		With '-R', and two arguments the given repository is replaced with the new name `,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				if len(args) == 2 && opts.flagRepo {
					opts.oldRepoSelector = args[0]
					opts.newRepoSelector = args[1]
				} else if len(args) == 1 && !opts.flagRepo {
					opts.newRepoSelector = args[0]
				} else {
					return fmt.Errorf("check your parameters")
				}
			} else {
				if !opts.IO.CanPrompt() {
					return &cmdutil.FlagError{
						Err: errors.New("could not prompt: proceed with prompt")}
				}
			}
			if runf != nil {
				return runf(opts)
			}
			return renameRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.flagRepo, "repo", "R", false, "pass in two arguments to rename a repository")

	return cmd
}

func renameRun(opts *RenameOptions) error {
	cs := opts.IO.ColorScheme()
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	var hostName string
	var input renameRepo

	if !opts.flagRepo {
		var newRepoName string
		currRepo, err := opts.BaseRepo()
		if err != nil {
			return err
		}

		hostName = currRepo.RepoHost()
		if opts.newRepoSelector != "" && opts.oldRepoSelector == "" {
			newRepoName = opts.newRepoSelector
		} else {
			err = prompt.SurveyAskOne(
				&survey.Input{
					Message: "Rename current repo to: ",
				},
				&newRepoName,
			)
			if err != nil {
				return err
			}
		}

		input = renameRepo{
			Owner:      currRepo.RepoOwner(),
			Repository: currRepo.RepoName(),
			Name:       newRepoName,
		}

	} else {
		oldRepoURL := opts.oldRepoSelector
		newRepoName := opts.newRepoSelector

		currentUser, err := api.CurrentLoginName(apiClient, ghinstance.Default())
		if err != nil {
			return err
		}

		if !strings.Contains(oldRepoURL, "/") {
			oldRepoURL = currentUser + "/" + oldRepoURL
		}

		currRepo, err := ghrepo.FromFullName(oldRepoURL)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}

		hostName = currRepo.RepoHost()

		input = renameRepo{
			Owner:      currRepo.RepoOwner(),
			Repository: currRepo.RepoName(),
			Name:       newRepoName,
		}
	}

	err = runRename(apiClient, hostName, input)
	if err != nil {
		return fmt.Errorf("API called failed: %s, please check your parameters", err)
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "%s Renamed repository %s\n", cs.SuccessIcon(), input.Owner+"/"+input.Name)
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
