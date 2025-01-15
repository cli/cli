package rename

import (
	"context"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
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
	Config          func() (gh.Config, error)
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
		Long: heredoc.Docf(`
			Rename a GitHub repository.
			
			%[1]s<new-name>%[1]s is the desired repository name without the owner.
			
			By default, the current repository is renamed. Otherwise, the repository specified
			with %[1]s--repo%[1]s is renamed.
			
			To transfer repository ownership to another user account or organization,
			you must follow additional steps on GitHub.com

			For more information on transferring repository ownership, see:
			<https://docs.github.com/en/repositories/creating-and-managing-repositories/transferring-a-repository>
			`, "`"),
		Example: heredoc.Doc(`
			# Rename the current repository (foo/bar -> foo/baz)
			$ gh repo rename baz

			# Rename the specified repository (qux/quux -> qux/baz)
			$ gh repo rename -R qux/quux baz
		`),
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

	cs := opts.IO.ColorScheme()
	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "%s Renamed repository %s\n", cs.SuccessIcon(), ghrepo.FullName(newRepo))
	}

	if opts.HasRepoOverride {
		return nil
	}

	remote, err := updateRemote(currRepo, newRepo, opts)
	if err != nil {
		if remote != nil {
			fmt.Fprintf(opts.IO.ErrOut, "%s Warning: unable to update remote %q: %v\n", cs.WarningIcon(), remote.Name, err)
		} else {
			fmt.Fprintf(opts.IO.ErrOut, "%s Warning: unable to update remote: %v\n", cs.WarningIcon(), err)
		}
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

	protocol := cfg.GitProtocol(repo.RepoHost()).Value

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
