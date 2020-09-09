package fork

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ForkOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)

	Repository   string
	Clone        bool
	Remote       bool
	PromptClone  bool
	PromptRemote bool
}

var Since = func(t time.Time) time.Duration {
	return time.Since(t)
}

func NewCmdFork(f *cmdutil.Factory, runF func(*ForkOptions) error) *cobra.Command {
	opts := &ForkOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		BaseRepo:   f.BaseRepo,
		Remotes:    f.Remotes,
	}

	cmd := &cobra.Command{
		Use:   "fork [<repository>]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Create a fork of a repository",
		Long: `Create a fork of a repository.

With no argument, creates a fork of the current repository. Otherwise, forks the specified repository.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			promptOk := opts.IO.CanPrompt()
			if len(args) > 0 {
				opts.Repository = args[0]
			}

			if promptOk && !cmd.Flags().Changed("clone") {
				opts.PromptClone = true
			}

			if promptOk && !cmd.Flags().Changed("remote") {
				opts.PromptRemote = true
			}

			if runF != nil {
				return runF(opts)
			}
			return forkRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Clone, "clone", false, "Clone the fork {true|false}")
	cmd.Flags().BoolVar(&opts.Remote, "remote", false, "Add remote for fork {true|false}")

	return cmd
}

func forkRun(opts *ForkOptions) error {
	var repoToFork ghrepo.Interface
	var err error
	inParent := false // whether or not we're forking the repo we're currently "in"
	if opts.Repository == "" {
		baseRepo, err := opts.BaseRepo()
		if err != nil {
			return fmt.Errorf("unable to determine base repository: %w", err)
		}
		inParent = true
		repoToFork = baseRepo
	} else {
		repoArg := opts.Repository

		if utils.IsURL(repoArg) {
			parsedURL, err := url.Parse(repoArg)
			if err != nil {
				return fmt.Errorf("did not understand argument: %w", err)
			}

			repoToFork, err = ghrepo.FromURL(parsedURL)
			if err != nil {
				return fmt.Errorf("did not understand argument: %w", err)
			}

		} else if strings.HasPrefix(repoArg, "git@") {
			parsedURL, err := git.ParseURL(repoArg)
			if err != nil {
				return fmt.Errorf("did not understand argument: %w", err)
			}
			repoToFork, err = ghrepo.FromURL(parsedURL)
			if err != nil {
				return fmt.Errorf("did not understand argument: %w", err)
			}
		} else {
			repoToFork, err = ghrepo.FromFullName(repoArg)
			if err != nil {
				return fmt.Errorf("argument error: %w", err)
			}
		}
	}

	connectedToTerminal := opts.IO.IsStdoutTTY() && opts.IO.IsStderrTTY() && opts.IO.IsStdinTTY()

	stderr := opts.IO.ErrOut
	s := utils.Spinner(stderr)
	stopSpinner := func() {}

	if connectedToTerminal {
		loading := utils.Gray("Forking ") + utils.Bold(utils.Gray(ghrepo.FullName(repoToFork))) + utils.Gray("...")
		s.Suffix = " " + loading
		s.FinalMSG = utils.Gray(fmt.Sprintf("- %s\n", loading))
		utils.StartSpinner(s)
		stopSpinner = func() {
			utils.StopSpinner(s)
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("unable to create client: %w", err)
	}

	apiClient := api.NewClientFromHTTP(httpClient)

	forkedRepo, err := api.ForkRepo(apiClient, repoToFork)
	if err != nil {
		stopSpinner()
		return fmt.Errorf("failed to fork: %w", err)
	}

	stopSpinner()

	// This is weird. There is not an efficient way to determine via the GitHub API whether or not a
	// given user has forked a given repo. We noticed, also, that the create fork API endpoint just
	// returns the fork repo data even if it already exists -- with no change in status code or
	// anything. We thus check the created time to see if the repo is brand new or not; if it's not,
	// we assume the fork already existed and report an error.
	createdAgo := Since(forkedRepo.CreatedAt)
	if createdAgo > time.Minute {
		if connectedToTerminal {
			fmt.Fprintf(stderr, "%s %s %s\n",
				utils.Yellow("!"),
				utils.Bold(ghrepo.FullName(forkedRepo)),
				"already exists")
		} else {
			fmt.Fprintf(stderr, "%s already exists", ghrepo.FullName(forkedRepo))
			return nil
		}
	} else {
		if connectedToTerminal {
			fmt.Fprintf(stderr, "%s Created fork %s\n", utils.GreenCheck(), utils.Bold(ghrepo.FullName(forkedRepo)))
		}
	}

	if (inParent && (!opts.Remote && !opts.PromptRemote)) || (!inParent && (!opts.Clone && !opts.PromptClone)) {
		return nil
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	protocol, err := cfg.Get(repoToFork.RepoHost(), "git_protocol")
	if err != nil {
		return err
	}

	if inParent {
		remotes, err := opts.Remotes()
		if err != nil {
			return err
		}
		if remote, err := remotes.FindByRepo(forkedRepo.RepoOwner(), forkedRepo.RepoName()); err == nil {
			if connectedToTerminal {
				fmt.Fprintf(stderr, "%s Using existing remote %s\n", utils.GreenCheck(), utils.Bold(remote.Name))
			}
			return nil
		}

		remoteDesired := opts.Remote
		if opts.PromptRemote {
			err = prompt.Confirm("Would you like to add a remote for the fork?", &remoteDesired)
			if err != nil {
				return fmt.Errorf("failed to prompt: %w", err)
			}
		}
		if remoteDesired {
			remoteName := "origin"

			remotes, err := opts.Remotes()
			if err != nil {
				return err
			}
			if _, err := remotes.FindByName(remoteName); err == nil {
				renameTarget := "upstream"
				renameCmd := git.GitCommand("remote", "rename", remoteName, renameTarget)
				err = run.PrepareCmd(renameCmd).Run()
				if err != nil {
					return err
				}
				if connectedToTerminal {
					fmt.Fprintf(stderr, "%s Renamed %s remote to %s\n", utils.GreenCheck(), utils.Bold(remoteName), utils.Bold(renameTarget))
				}
			}

			forkedRepoCloneURL := ghrepo.FormatRemoteURL(forkedRepo, protocol)

			_, err = git.AddRemote(remoteName, forkedRepoCloneURL)
			if err != nil {
				return fmt.Errorf("failed to add remote: %w", err)
			}

			if connectedToTerminal {
				fmt.Fprintf(stderr, "%s Added remote %s\n", utils.GreenCheck(), utils.Bold(remoteName))
			}
		}
	} else {
		cloneDesired := opts.Clone
		if opts.PromptClone {
			err = prompt.Confirm("Would you like to clone the fork?", &cloneDesired)
			if err != nil {
				return fmt.Errorf("failed to prompt: %w", err)
			}
		}
		if cloneDesired {
			forkedRepoURL := ghrepo.FormatRemoteURL(forkedRepo, protocol)
			cloneDir, err := git.RunClone(forkedRepoURL, []string{})
			if err != nil {
				return fmt.Errorf("failed to clone fork: %w", err)
			}

			upstreamURL := ghrepo.FormatRemoteURL(repoToFork, protocol)
			err = git.AddUpstreamRemote(upstreamURL, cloneDir)
			if err != nil {
				return err
			}

			if connectedToTerminal {
				fmt.Fprintf(stderr, "%s Cloned fork\n", utils.GreenCheck())
			}
		}
	}

	return nil
}
