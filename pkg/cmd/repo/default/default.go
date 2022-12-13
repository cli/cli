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

type iprompter interface {
	Select(string, string, []string) (int, error)
}

type DefaultOptions struct {
	IO         *iostreams.IOStreams
	Remotes    func() (context.Remotes, error)
	HttpClient func() (*http.Client, error)
	Prompter   iprompter

	Repo     ghrepo.Interface
	ViewMode bool
}

func NewCmdDefault(f *cmdutil.Factory, runF func(*DefaultOptions) error) *cobra.Command {
	opts := &DefaultOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Remotes:    f.Remotes,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "default [<repository>]",
		Short: "Configure default repository",
		Long: heredoc.Docf(`
			Set default repository for current directory.

			The default repository is used as the target
			repository for various commands such as %[1]spr%[1]s, %[1]sissue%[1]s,
			and %[1]srepo%[1]s.
		`, "`"),
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

			c := &git.Client{}

			if !c.InGitDirectory(ctx.Background()) {
				return errors.New("must be run from inside a git repository")
			}

			if runF != nil {
				return runF(opts)
			}

			return defaultRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.ViewMode, "view", "v", false, "view the current default repository")

	return cmd
}

func defaultRun(opts *DefaultOptions) error {
	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}

	currentDefaultRepo, _ := remotes.ResolvedRemote()

	if opts.ViewMode {
		if currentDefaultRepo == nil {
			fmt.Fprintln(opts.IO.Out, "no default repo has been set; use `gh repo default` to select one")
		} else {
			fmt.Fprintln(opts.IO.Out, displayRemoteRepoName(currentDefaultRepo))
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

	knownRepos, err := resolvedRemotes.NetworkRepos()
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
	cs := opts.IO.ColorScheme()

	hintMsg := cs.Gray("(hint: for gh to see more remote repositories, add them with `git remote`)")

	if selectedRepo == nil {
		if len(knownRepos) == 1 {
			selectedRepo = knownRepos[0]

			fmt.Fprintf(opts.IO.Out, "Found only one known remote repo, %s on %s.\n",
				cs.Bold(ghrepo.FullName(selectedRepo)),
				cs.Bold(selectedRepo.RepoHost()))
			fmt.Fprintln(opts.IO.Out, hintMsg)
			fmt.Fprintln(opts.IO.Out)
		} else {
			var repoNames []string
			current := ""
			if currentDefaultRepo != nil {
				current = ghrepo.FullName(currentDefaultRepo)
			}

			for _, knownRepo := range knownRepos {
				repoNames = append(repoNames, ghrepo.FullName(knownRepo))
			}

			defaultExplainer := heredoc.Doc(`
				gh uses the default repository for things like:
				
				 - viewing, creating, and setting the default base for  pull requests
				 - viewing and creating issues
				 - viewing and creating releases
				 - working with Actions
				 - adding secrets
			`)

			fmt.Fprintln(opts.IO.Out, defaultExplainer)
			fmt.Fprintln(opts.IO.Out, hintMsg)
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
		if err := unsetDefaultRepo(currentDefaultRepo); err != nil {
			return err
		}
	}

	err = setDefaultRepo(selectedRemote, resolution)
	if err != nil {
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

func setDefaultRepo(remote *context.Remote, resolution string) error {
	c := &git.Client{}
	return c.SetRemoteResolution(ctx.Background(), remote.Name, resolution)
}

func unsetDefaultRepo(remote *context.Remote) error {
	c := &git.Client{}
	return c.UnsetRemoteResolution(ctx.Background(), remote.Name)
}
