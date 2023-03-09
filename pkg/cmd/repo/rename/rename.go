package rename

import (
	"context"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type iprompter interface {
	Input(string, string) (string, error)
	Confirm(string, bool) (bool, error)
}

type RenameOptions struct {
	HttpClient      func() (*http.Client, error)
	GitClient       *git.Client
	IO              *iostreams.IOStreams
	Prompter        iprompter
	Config          func() (config.Config, error)
	BaseRepo        func() (ghrepo.Interface, error)
	Remotes         func() (ghContext.Remotes, error)
	DoConfirm       bool
	HasRepoOverride bool
	newRepoSelector string
}

func NewCmdRename(f *cmdutil.Factory, runf func(*RenameOptions) error) *cobra.Command {
	opts := &RenameOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Remotes:    f.Remotes,
		Config:     f.Config,
		Prompter:   f.Prompter,
	}

	var confirm bool

	cmd := &cobra.Command{
		Use:   "rename [<new-name>]",
		Short: "Rename a repository",
		Long: heredoc.Doc(`Rename a GitHub repository.

		By default, this renames the current repository; otherwise renames the specified repository.`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.HasRepoOverride = cmd.Flags().Changed("repo")

			if len(args) > 0 {
				opts.newRepoSelector = args[0]
			} else if !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("new name argument required when not running interactively")
			}

			if len(args) == 1 && !confirm && !opts.HasRepoOverride {
				if !opts.IO.CanPrompt() {
					return cmdutil.FlagErrorf("--yes required when passing a single argument")
				}
				opts.DoConfirm = true
			}

			if runf != nil {
				return runf(opts)
			}

			return renameRun(opts)
		},
	}

	cmdutil.EnableRepoOverride(cmd, f)
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Skip confirmation prompt")
	_ = cmd.Flags().MarkDeprecated("confirm", "use `--yes` instead")
	cmd.Flags().BoolVarP(&confirm, "yes", "y", false, "Skip the confirmation prompt")

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
		if newRepoName, err = opts.Prompter.Input(fmt.Sprintf(
			"Rename %s to:", ghrepo.FullName(currRepo)), ""); err != nil {
			return err
		}
	}

	if opts.DoConfirm {
		var confirmed bool
		if confirmed, err = opts.Prompter.Confirm(fmt.Sprintf(
			"Rename %s to %s?", ghrepo.FullName(currRepo), newRepoName), false); err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	apiClient := api.NewClientFromHTTP(httpClient)

	newRepo, err := api.RenameRepo(apiClient, currRepo, newRepoName)
	if err != nil {
		return err
	}

	renamedRepo := ghrepo.New(newRepo.Owner.Login, newRepo.Name)

	cs := opts.IO.ColorScheme()
	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "%s Renamed repository %s\n", cs.SuccessIcon(), ghrepo.FullName(newRepo))
	}

	if opts.HasRepoOverride {
		return nil
	}

	remote, err := updateRemote(currRepo, renamedRepo, opts)
	if err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "%s Warning: unable to update remote %q: %v\n", cs.WarningIcon(), remote.Name, err)
	} else if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "%s Updated the %q remote\n", cs.SuccessIcon(), remote.Name)
	}

	return nil
}

func updateRemote(repo ghrepo.Interface, renamed ghrepo.Interface, opts *RenameOptions) (*ghContext.Remote, error) {
	cfg, err := opts.Config()
	if err != nil {
		return nil, err
	}

	protocol, err := cfg.GetOrDefault(repo.RepoHost(), "git_protocol")
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
	err = opts.GitClient.UpdateRemoteURL(context.Background(), remote.Name, remoteURL)

	return remote, err
}
