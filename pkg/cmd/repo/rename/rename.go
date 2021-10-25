package rename

import (
	"fmt"
	"net/http"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
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
	HasRepoOverride bool
	newRepoSelector string
}

func NewCmdRename(f *cmdutil.Factory, runf func(*RenameOptions) error) *cobra.Command {
	opts := &RenameOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Remotes:    f.Remotes,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "rename [<new-name>]",
		Short: "Rename a repository",
		Long: heredoc.Doc(`Rename a GitHub repository

		By default, renames the current repository otherwise rename the specified repository.`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.HasRepoOverride = cmd.Flags().Changed("repo")

			if len(args) > 0 {
				opts.newRepoSelector = args[0]
			} else if !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("could not prompt: new name required when not running interactively")
			}

			if runf != nil {
				return runf(opts)
			}
			return renameRun(opts)
		},
	}

	cmdutil.EnableRepoOverride(cmd, f)

	return cmd
}

func renameRun(opts *RenameOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	var newRepo ghrepo.Interface
	var baseRemote *context.Remote
	newRepoName := opts.newRepoSelector

	currRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	if newRepoName == "" {
		err = prompt.SurveyAskOne(
			&survey.Input{
				Message: fmt.Sprintf("Rename %s to: ", ghrepo.FullName(currRepo)),
			},
			&newRepoName,
		)
		if err != nil {
			return err
		}
	}

	newRepo = ghrepo.NewWithHost(currRepo.RepoOwner(), newRepoName, currRepo.RepoHost())

	err = runRename(httpClient, currRepo, newRepoName)
	if err != nil {
		return fmt.Errorf("API called failed: %s", err)
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s Renamed repository %s\n", cs.SuccessIcon(), ghrepo.FullName(newRepo))
	}

	if !opts.HasRepoOverride {
		cs := opts.IO.ColorScheme()
		cfg, err := opts.Config()
		if err != nil {
			return err
		}

		protocol, _ := cfg.Get(currRepo.RepoHost(), "git_protocol")
		remotes, _ := opts.Remotes()
		baseRemote, _ = remotes.FindByRepo(currRepo.RepoOwner(), currRepo.RepoName())
		remoteURL := ghrepo.FormatRemoteURL(newRepo, protocol)
		err = git.UpdateRemoteURL(baseRemote.Name, remoteURL)
		if err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "%s warning: unable to update remote '%s' \n", cs.WarningIcon(), err)
		}
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "%s Updated the %q remote \n", cs.SuccessIcon(), baseRemote.Name)
		}
	}
	return nil
}
