package rename

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
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
	Remotes         func() (context.Remotes, error)
	oldRepoSelector string
	newRepoSelector string
}

type renameRepo struct {
	RepoHost  string
	RepoOwner string
	RepoName  string
	Name      string `json:"name,omitempty"`
}

func NewCmdRename(f *cmdutil.Factory, runf func(*RenameOptions) error) *cobra.Command {
	opts := &RenameOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
		Remotes:    f.Remotes,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "rename [-R] [<repository>] [<new-name>]",
		Short: "Rename a repository",
		Long: `Rename a GitHub repository
		With no argument, the repository for the current directory is renamed using a prompt
		With one argument, the repository of the current directory is renamed using the argument
		With '-R', and two arguments the given repository is replaced with the new name `,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.newRepoSelector = args[0]
			} else {
				if !opts.IO.CanPrompt() {
					return &cmdutil.FlagError{
						Err: errors.New("could not prompt: proceed with prompt")}
				}
			}
			return renameRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.oldRepoSelector, "repo", "R", "", "pass in two arguments to rename a repository")

	return cmd
}

func renameRun(opts *RenameOptions) error {
	cs := opts.IO.ColorScheme()
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	var input renameRepo
	var oldRepo ghrepo.Interface
	var newRepo ghrepo.Interface

	if opts.oldRepoSelector == "" {
		var newRepoName string
		currRepo, err := opts.BaseRepo()
		if err != nil {
			return err
		}

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

		oldRepo = ghrepo.NewWithHost(currRepo.RepoOwner(), currRepo.RepoName(), currRepo.RepoHost())
		newRepo = ghrepo.NewWithHost(currRepo.RepoOwner(), newRepoName, currRepo.RepoHost())

		input = renameRepo{
			RepoHost:  currRepo.RepoHost(),
			RepoOwner: currRepo.RepoOwner(),
			RepoName:  currRepo.RepoName(),
			Name:      newRepoName,
		}

	} else {
		oldRepoURL := opts.oldRepoSelector
		newRepoName := opts.newRepoSelector

		currRepo, err := ghrepo.FromFullName(oldRepoURL)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}

		oldRepo = ghrepo.NewWithHost(currRepo.RepoOwner(), currRepo.RepoName(), currRepo.RepoHost())

		input = renameRepo{
			RepoHost:  currRepo.RepoHost(),
			RepoOwner: currRepo.RepoOwner(),
			RepoName:  currRepo.RepoName(),
			Name:      newRepoName,
		}
	}

	err = runRename(apiClient, oldRepo.RepoHost(), input)
	if err != nil {
		return fmt.Errorf("API called failed: %s, please check your parameters", err)
	}

	if opts.oldRepoSelector == "" {
		cfg, err := opts.Config()
		if err != nil {
			return err
		}

		protocol, err := cfg.Get(oldRepo.RepoHost(), "git_protocol")
		if err != nil {
			return err
		}

		remotes, err := opts.Remotes()
		if err != nil {
			return err
		}

		baseRemote, err := remotes.FindByRepo(oldRepo.RepoOwner(), oldRepo.RepoName())
		if err != nil {
			return err
		}

		remoteURL := ghrepo.FormatRemoteURL(newRepo, protocol)
		err = git.UpdateRemoteURL(baseRemote.Name, remoteURL)
		if err != nil {
			return err
		}
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "%s Renamed repository %s\n", cs.SuccessIcon(), input.RepoOwner+"/"+input.Name)
		if opts.oldRepoSelector == "" {
			fmt.Fprintf(opts.IO.Out, `%s Updated the "origin" remote`, cs.SuccessIcon())
		}
	}

	return nil
}

func runRename(apiClient *api.Client, hostname string, input renameRepo) error {
	path := fmt.Sprintf("repos/%s/%s", input.RepoOwner, input.RepoName)
	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	if err := enc.Encode(input); err != nil {
		return err
	}

	return apiClient.REST(hostname, "PATCH", path, body, nil)
}
