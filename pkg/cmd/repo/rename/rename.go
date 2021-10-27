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

		By default, renames the current repository otherwise renames the specified repository.`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.HasRepoOverride = cmd.Flags().Changed("repo")

			if len(args) > 0 {
				opts.newRepoSelector = args[0]
			} else if !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("new name argument required when not running interactively\n")
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

	err = runRename(httpClient, currRepo, newRepoName)
	if err != nil {
		return fmt.Errorf("API called failed: %s", err)
	}

	cs := opts.IO.ColorScheme()
	newRepo := ghrepo.NewWithHost(currRepo.RepoOwner(), newRepoName, currRepo.RepoHost())

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "%s Renamed repository %s\n", cs.SuccessIcon(), ghrepo.FullName(newRepo))
	}

	if !opts.HasRepoOverride {
		remote, err := updateRemote(currRepo, newRepo, opts)
		if err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "%s Warning: unable to update remote %q\n", cs.WarningIcon(), remote.Name)
		} else if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "%s Updated the %q remote\n", cs.SuccessIcon(), remote.Name)
		}
	}

	return nil
}

func updateRemote(repo ghrepo.Interface, renamed ghrepo.Interface, opts *RenameOptions) (*context.Remote, error) {
	cfg, err := opts.Config()
	if err != nil {
		return nil, err
	}

	protocol, err := cfg.Get(repo.RepoHost(), "git_protocol")
	if err != nil {
		return nil, err
	}

	remotes, err := opts.Remotes()
	if err != nil {
		return nil, err
	}

	remote, err := remotes.FindByRepo(repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return nil, err
	}

	remoteURL := ghrepo.FormatRemoteURL(renamed, protocol)
	err = git.UpdateRemoteURL(remote.Name, remoteURL)
	return remote, err
}
