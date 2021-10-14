package fork

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const defaultRemoteName = "origin"

type ForkOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)
	Since      func(time.Time) time.Duration

	GitArgs      []string
	Repository   string
	Clone        bool
	Remote       bool
	PromptClone  bool
	PromptRemote bool
	RemoteName   string
	Organization string
	Rename       bool
}

// TODO warn about useless flags (--remote, --remote-name) when running from outside a repository
// TODO output over STDOUT not STDERR
// TODO remote-name has no effect on its own; error that or change behavior

func NewCmdFork(f *cmdutil.Factory, runF func(*ForkOptions) error) *cobra.Command {
	opts := &ForkOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		BaseRepo:   f.BaseRepo,
		Remotes:    f.Remotes,
		Since:      time.Since,
	}

	cmd := &cobra.Command{
		Use: "fork [<repository>] [-- <gitflags>...]",
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.ArgsLenAtDash() == 0 && len(args[1:]) > 0 {
				return cmdutil.FlagError{Err: fmt.Errorf("repository argument required when passing 'git clone' flags")}
			}
			return nil
		},
		Short: "Create a fork of a repository",
		Long: `Create a fork of a repository.

With no argument, creates a fork of the current repository. Otherwise, forks
the specified repository.

By default, the new fork is set to be your 'origin' remote and any existing
origin remote is renamed to 'upstream'. To alter this behavior, you can set
a name for the new fork's remote with --remote-name.

Additional 'git clone' flags can be passed in by listing them after '--'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			promptOk := opts.IO.CanPrompt()
			if len(args) > 0 {
				opts.Repository = args[0]
				opts.GitArgs = args[1:]
			}

			if cmd.Flags().Changed("org") && opts.Organization == "" {
				return &cmdutil.FlagError{Err: errors.New("--org cannot be blank")}
			}

			if opts.RemoteName == "" {
				return &cmdutil.FlagError{Err: errors.New("--remote-name cannot be blank")}
			} else if !cmd.Flags().Changed("remote-name") {
				opts.Rename = true // Any existing 'origin' will be renamed to upstream
			}

			if promptOk {
				// We can prompt for these if they were not specified.
				opts.PromptClone = !cmd.Flags().Changed("clone")
				opts.PromptRemote = !cmd.Flags().Changed("remote")
			}

			if runF != nil {
				return runF(opts)
			}
			return forkRun(opts)
		},
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		if err == pflag.ErrHelp {
			return err
		}
		return &cmdutil.FlagError{Err: fmt.Errorf("%w\nSeparate git clone flags with '--'.", err)}
	})

	cmd.Flags().BoolVar(&opts.Clone, "clone", false, "Clone the fork {true|false}")
	cmd.Flags().BoolVar(&opts.Remote, "remote", false, "Add remote for fork {true|false}")
	cmd.Flags().StringVar(&opts.RemoteName, "remote-name", defaultRemoteName, "Specify a name for a fork's new remote.")
	cmd.Flags().StringVar(&opts.Organization, "org", "", "Create the fork in an organization")

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

	cs := opts.IO.ColorScheme()
	stderr := opts.IO.ErrOut

	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("unable to create client: %w", err)
	}

	apiClient := api.NewClientFromHTTP(httpClient)

	opts.IO.StartProgressIndicator()
	forkedRepo, err := api.ForkRepo(apiClient, repoToFork, opts.Organization)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("failed to fork: %w", err)
	}

	// This is weird. There is not an efficient way to determine via the GitHub API whether or not a
	// given user has forked a given repo. We noticed, also, that the create fork API endpoint just
	// returns the fork repo data even if it already exists -- with no change in status code or
	// anything. We thus check the created time to see if the repo is brand new or not; if it's not,
	// we assume the fork already existed and report an error.
	createdAgo := opts.Since(forkedRepo.CreatedAt)
	if createdAgo > time.Minute {
		if connectedToTerminal {
			fmt.Fprintf(stderr, "%s %s %s\n",
				cs.Yellow("!"),
				cs.Bold(ghrepo.FullName(forkedRepo)),
				"already exists")
		} else {
			fmt.Fprintf(stderr, "%s already exists", ghrepo.FullName(forkedRepo))
		}
	} else {
		if connectedToTerminal {
			fmt.Fprintf(stderr, "%s Created fork %s\n", cs.SuccessIconWithColor(cs.Green), cs.Bold(ghrepo.FullName(forkedRepo)))
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

		if remote, err := remotes.FindByRepo(repoToFork.RepoOwner(), repoToFork.RepoName()); err == nil {

			scheme := ""
			if remote.FetchURL != nil {
				scheme = remote.FetchURL.Scheme
			}
			if remote.PushURL != nil {
				scheme = remote.PushURL.Scheme
			}
			if scheme != "" {
				protocol = scheme
			}
		}

		if remote, err := remotes.FindByRepo(forkedRepo.RepoOwner(), forkedRepo.RepoName()); err == nil {
			if connectedToTerminal {
				fmt.Fprintf(stderr, "%s Using existing remote %s\n", cs.SuccessIcon(), cs.Bold(remote.Name))
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
			remoteName := opts.RemoteName
			remotes, err := opts.Remotes()
			if err != nil {
				return err
			}

			if _, err := remotes.FindByName(remoteName); err == nil {
				if opts.Rename {
					renameTarget := "upstream"
					renameCmd, err := git.GitCommand("remote", "rename", remoteName, renameTarget)
					if err != nil {
						return err
					}
					err = run.PrepareCmd(renameCmd).Run()
					if err != nil {
						return err
					}
				} else {
					return fmt.Errorf("a git remote named '%s' already exists", remoteName)
				}
			}

			forkedRepoCloneURL := ghrepo.FormatRemoteURL(forkedRepo, protocol)

			_, err = git.AddRemote(remoteName, forkedRepoCloneURL)
			if err != nil {
				return fmt.Errorf("failed to add remote: %w", err)
			}

			if connectedToTerminal {
				fmt.Fprintf(stderr, "%s Added remote %s\n", cs.SuccessIcon(), cs.Bold(remoteName))
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
			cloneDir, err := git.RunClone(forkedRepoURL, opts.GitArgs)
			if err != nil {
				return fmt.Errorf("failed to clone fork: %w", err)
			}

			upstreamURL := ghrepo.FormatRemoteURL(repoToFork, protocol)
			err = git.AddUpstreamRemote(upstreamURL, cloneDir, []string{})
			if err != nil {
				return err
			}

			if connectedToTerminal {
				fmt.Fprintf(stderr, "%s Cloned fork\n", cs.SuccessIcon())
			}
		}
	}

	return nil
}
