package fork

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cenkalti/backoff/v4"
	"github.com/cli/cli/v2/api"
	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const defaultRemoteName = "origin"

type iprompter interface {
	Confirm(string, bool) (bool, error)
}

type ForkOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (ghContext.Remotes, error)
	Since      func(time.Time) time.Duration
	BackOff    backoff.BackOff
	Prompter   iprompter

	GitArgs           []string
	Repository        string
	Clone             bool
	Remote            bool
	PromptClone       bool
	PromptRemote      bool
	RemoteName        string
	Organization      string
	ForkName          string
	Rename            bool
	DefaultBranchOnly bool
}

type errWithExitCode interface {
	ExitCode() int
}

// TODO warn about useless flags (--remote, --remote-name) when running from outside a repository
// TODO output over STDOUT not STDERR
// TODO remote-name has no effect on its own; error that or change behavior

func NewCmdFork(f *cmdutil.Factory, runF func(*ForkOptions) error) *cobra.Command {
	opts := &ForkOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Config:     f.Config,
		BaseRepo:   f.BaseRepo,
		Remotes:    f.Remotes,
		Prompter:   f.Prompter,
		Since:      time.Since,
	}

	cmd := &cobra.Command{
		Use: "fork [<repository>] [-- <gitflags>...]",
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.ArgsLenAtDash() == 0 && len(args[1:]) > 0 {
				return cmdutil.FlagErrorf("repository argument required when passing git clone flags")
			}
			return nil
		},
		Short: "Create a fork of a repository",
		Long: heredoc.Docf(`
			Create a fork of a repository.

			With no argument, creates a fork of the current repository. Otherwise, forks
			the specified repository.

			By default, the new fork is set to be your %[1]sorigin%[1]s remote and any existing
			origin remote is renamed to %[1]supstream%[1]s. To alter this behavior, you can set
			a name for the new fork's remote with %[1]s--remote-name%[1]s.

			The %[1]supstream%[1]s remote will be set as the default remote repository.

			Additional %[1]sgit clone%[1]s flags can be passed after %[1]s--%[1]s.
		`, "`"),
		RunE: func(cmd *cobra.Command, args []string) error {
			promptOk := opts.IO.CanPrompt()
			if len(args) > 0 {
				opts.Repository = args[0]
				opts.GitArgs = args[1:]
			}

			if cmd.Flags().Changed("org") && opts.Organization == "" {
				return cmdutil.FlagErrorf("--org cannot be blank")
			}

			if opts.RemoteName == "" {
				return cmdutil.FlagErrorf("--remote-name cannot be blank")
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
		return cmdutil.FlagErrorf("%w\nSeparate git clone flags with `--`.", err)
	})

	cmd.Flags().BoolVar(&opts.Clone, "clone", false, "Clone the fork")
	cmd.Flags().BoolVar(&opts.Remote, "remote", false, "Add a git remote for the fork")
	cmd.Flags().StringVar(&opts.RemoteName, "remote-name", defaultRemoteName, "Specify the name for the new remote")
	cmd.Flags().StringVar(&opts.Organization, "org", "", "Create the fork in an organization")
	cmd.Flags().StringVar(&opts.ForkName, "fork-name", "", "Rename the forked repository")
	cmd.Flags().BoolVar(&opts.DefaultBranchOnly, "default-branch-only", false, "Only include the default branch in the fork")

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

		if isURL(repoArg) {
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
	forkedRepo, err := api.ForkRepo(apiClient, repoToFork, opts.Organization, opts.ForkName, opts.DefaultBranchOnly)
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
			fmt.Fprintf(stderr, "%s already exists\n", ghrepo.FullName(forkedRepo))
		}
	} else {
		if connectedToTerminal {
			fmt.Fprintf(stderr, "%s Created fork %s\n", cs.SuccessIconWithColor(cs.Green), cs.Bold(ghrepo.FullName(forkedRepo)))
		} else {
			fmt.Fprintln(opts.IO.Out, ghrepo.GenerateRepoURL(forkedRepo, ""))
		}
	}

	// Rename the new repo if necessary
	if opts.ForkName != "" && !strings.EqualFold(forkedRepo.RepoName(), shared.NormalizeRepoName(opts.ForkName)) {
		forkedRepo, err = api.RenameRepo(apiClient, forkedRepo, opts.ForkName)
		if err != nil {
			return fmt.Errorf("could not rename fork: %w", err)
		}
		if connectedToTerminal {
			fmt.Fprintf(stderr, "%s Renamed fork to %s\n", cs.SuccessIconWithColor(cs.Green), cs.Bold(ghrepo.FullName(forkedRepo)))
		}
	}

	if (inParent && (!opts.Remote && !opts.PromptRemote)) || (!inParent && (!opts.Clone && !opts.PromptClone)) {
		return nil
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	protocolConfig := cfg.GitProtocol(repoToFork.RepoHost())
	protocolIsConfiguredByUser := protocolConfig.Source == gh.ConfigUserProvided
	protocol := protocolConfig.Value

	gitClient := opts.GitClient
	ctx := context.Background()

	if inParent {
		remotes, err := opts.Remotes()
		if err != nil {
			return err
		}

		if !protocolIsConfiguredByUser {
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
				} else {
					protocol = "https"
				}
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
			remoteDesired, err = opts.Prompter.Confirm("Would you like to add a remote for the fork?", false)
			if err != nil {
				return err
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
					renameCmd, err := gitClient.Command(ctx, "remote", "rename", remoteName, renameTarget)
					if err != nil {
						return err
					}
					_, err = renameCmd.Output()
					if err != nil {
						return err
					}

					if connectedToTerminal {
						fmt.Fprintf(stderr, "%s Renamed remote %s to %s\n", cs.SuccessIcon(), cs.Bold(remoteName), cs.Bold(renameTarget))
					}
				} else {
					return fmt.Errorf("a git remote named '%s' already exists", remoteName)
				}
			}

			forkedRepoCloneURL := ghrepo.FormatRemoteURL(forkedRepo, protocol)

			_, err = gitClient.AddRemote(ctx, remoteName, forkedRepoCloneURL, []string{})
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
			cloneDesired, err = opts.Prompter.Confirm("Would you like to clone the fork?", false)
			if err != nil {
				return err
			}
		}
		if cloneDesired {
			// Allow injecting alternative BackOff in tests.
			if opts.BackOff == nil {
				bo := backoff.NewConstantBackOff(2 * time.Second)
				opts.BackOff = bo
			}

			cloneDir, err := backoff.RetryWithData(func() (string, error) {
				forkedRepoURL := ghrepo.FormatRemoteURL(forkedRepo, protocol)
				dir, err := gitClient.Clone(ctx, forkedRepoURL, opts.GitArgs)
				if err == nil {
					return dir, err
				}
				var execError errWithExitCode
				if errors.As(err, &execError) && execError.ExitCode() == 128 {
					return "", err
				}
				return "", backoff.Permanent(err)
			}, backoff.WithContext(backoff.WithMaxRetries(opts.BackOff, 3), ctx))

			if err != nil {
				return fmt.Errorf("failed to clone fork: %w", err)
			}

			gc := gitClient.Copy()
			gc.RepoDir = cloneDir
			upstreamURL := ghrepo.FormatRemoteURL(repoToFork, protocol)
			upstreamRemote := "upstream"

			if _, err := gc.AddRemote(ctx, upstreamRemote, upstreamURL, []string{}); err != nil {
				return err
			}

			if err := gc.SetRemoteResolution(ctx, upstreamRemote, "base"); err != nil {
				return err
			}

			if err := gc.Fetch(ctx, upstreamRemote, ""); err != nil {
				return err
			}

			if connectedToTerminal {
				fmt.Fprintf(stderr, "%s Cloned fork\n", cs.SuccessIcon())
				fmt.Fprintf(stderr, "%s Repository %s set as the default repository. To learn more about the default repository, run: gh repo set-default --help\n", cs.WarningIcon(), cs.Bold(ghrepo.FullName(repoToFork)))
			}
		}
	}

	return nil
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http:/") || strings.HasPrefix(s, "https:/")
}
