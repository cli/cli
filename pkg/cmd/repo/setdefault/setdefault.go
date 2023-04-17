package base

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	ctx "context"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

func explainer() string {
	return heredoc.Doc(`
		This command sets the default remote repository to use when querying the
		GitHub API for the locally cloned repository.

		gh uses the default repository for things like:

		 - viewing and creating pull requests
		 - viewing and creating issues
		 - viewing and creating releases
		 - working with Actions
		 - adding repository and environment secrets`)
}

type iprompter interface {
	Select(string, string, []string) (int, error)
}

type SetDefaultOptions struct {
	IO         *iostreams.IOStreams
	Remotes    func() (context.Remotes, error)
	HttpClient func() (*http.Client, error)
	Prompter   iprompter
	GitClient  *git.Client

	Repo      ghrepo.Interface
	ViewMode  bool
	UnsetMode bool
}

func NewCmdSetDefault(f *cmdutil.Factory, runF func(*SetDefaultOptions) error) *cobra.Command {
	opts := &SetDefaultOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Remotes:    f.Remotes,
		Prompter:   f.Prompter,
		GitClient:  f.GitClient,
	}

	cmd := &cobra.Command{
		Use:   "set-default [<repository>]",
		Short: "Configure default repository for this directory",
		Long:  explainer(),
		Example: heredoc.Doc(`
			Interactively select a default repository:
			$ gh repo set-default

			Set a repository explicitly:
			$ gh repo set-default owner/repo

			View the current default repository:
			$ gh repo set-default --view

			Show more repository options in the interactive picker:
			$ git remote add newrepo https://github.com/owner/repo
			$ gh repo set-default
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				var err error
				opts.Repo, err = ghrepo.FromFullName(args[0])
				if err != nil {
					return err
				}
			}

			if !opts.IO.CanPrompt() && opts.Repo == nil {
				return cmdutil.FlagErrorf("repository required when not running interactively")
			}

			if isLocal, err := opts.GitClient.IsLocalGitRepo(cmd.Context()); err != nil {
				return err
			} else if !isLocal {
				return errors.New("must be run from inside a git repository")
			}

			if runF != nil {
				return runF(opts)
			}

			return setDefaultRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.ViewMode, "view", "v", false, "view the current default repository")
	cmd.Flags().BoolVarP(&opts.UnsetMode, "unset", "u", false, "unset the current default repository")

	return cmd
}

func setDefaultRun(opts *SetDefaultOptions) error {
	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}

	currentDefaultRepo, _ := remotes.ResolvedRemote()

	if opts.ViewMode {
		if currentDefaultRepo == nil {
			fmt.Fprintln(opts.IO.Out, "no default repository has been set; use `gh repo set-default` to select one")
		} else {
			fmt.Fprintln(opts.IO.Out, displayRemoteRepoName(currentDefaultRepo))
		}
		return nil
	}

	cs := opts.IO.ColorScheme()

	if opts.UnsetMode {
		var msg string
		if currentDefaultRepo != nil {
			if err := opts.GitClient.UnsetRemoteResolution(
				ctx.Background(), currentDefaultRepo.Name); err != nil {
				return err
			}
			msg = fmt.Sprintf("%s Unset %s as default repository",
				cs.SuccessIcon(), ghrepo.FullName(currentDefaultRepo))
		} else {
			msg = "no default repository has been set"
		}

		if opts.IO.IsStdoutTTY() {
			fmt.Fprintln(opts.IO.Out, msg)
		}

		return nil
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	resolvedRemotes, err := context.ResolveRemotesToRepos(remotes, apiClient, "")
	if err != nil {
		return err
	}

	knownRepos, err := resolvedRemotes.NetworkRepos(0)
	if err != nil {
		return err
	}
	if len(knownRepos) == 0 {
		return errors.New("none of the git remotes correspond to a valid remote repository")
	}

	var selectedRepo ghrepo.Interface

	if opts.Repo != nil {
		for _, knownRepo := range knownRepos {
			if ghrepo.IsSame(opts.Repo, knownRepo) {
				selectedRepo = opts.Repo
				break
			}
		}
		if selectedRepo == nil {
			return fmt.Errorf("%s does not correspond to any git remotes", ghrepo.FullName(opts.Repo))
		}
	}

	if selectedRepo == nil {
		if len(knownRepos) == 1 {
			selectedRepo = knownRepos[0]

			fmt.Fprintf(opts.IO.Out, "Found only one known remote repo, %s on %s.\n",
				cs.Bold(ghrepo.FullName(selectedRepo)),
				cs.Bold(selectedRepo.RepoHost()))
		} else {
			var repoNames []string
			current := ""
			if currentDefaultRepo != nil {
				current = ghrepo.FullName(currentDefaultRepo)
			}

			for _, knownRepo := range knownRepos {
				repoNames = append(repoNames, ghrepo.FullName(knownRepo))
			}

			fmt.Fprintln(opts.IO.Out, explainer())
			fmt.Fprintln(opts.IO.Out)

			selected, err := opts.Prompter.Select("Which repository should be the default?", current, repoNames)
			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}
			selectedName := repoNames[selected]

			owner, repo, _ := strings.Cut(selectedName, "/")
			selectedRepo = ghrepo.New(owner, repo)
		}
	}

	resolution := "base"
	selectedRemote, _ := resolvedRemotes.RemoteForRepo(selectedRepo)
	if selectedRemote == nil {
		sort.Stable(remotes)
		selectedRemote = remotes[0]
		resolution = ghrepo.FullName(selectedRepo)
	}

	if currentDefaultRepo != nil {
		if err := opts.GitClient.UnsetRemoteResolution(
			ctx.Background(), currentDefaultRepo.Name); err != nil {
			return err
		}
	}
	if err = opts.GitClient.SetRemoteResolution(
		ctx.Background(), selectedRemote.Name, resolution); err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s Set %s as the default repository for the current directory\n", cs.SuccessIcon(), ghrepo.FullName(selectedRepo))
	}

	return nil
}

func displayRemoteRepoName(remote *context.Remote) string {
	if remote.Resolved == "" || remote.Resolved == "base" {
		return ghrepo.FullName(remote)
	}

	repo, err := ghrepo.FromFullName(remote.Resolved)
	if err != nil {
		return ghrepo.FullName(remote)
	}

	return ghrepo.FullName(repo)
}
