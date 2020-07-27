package command

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func init() {
	repoCmd.AddCommand(repoForkCmd)
	repoForkCmd.Flags().String("clone", "prompt", "Clone fork: {true|false|prompt}")
	repoForkCmd.Flags().String("remote", "prompt", "Add remote for fork: {true|false|prompt}")
	repoForkCmd.Flags().Lookup("clone").NoOptDefVal = "true"
	repoForkCmd.Flags().Lookup("remote").NoOptDefVal = "true"

	repoCmd.AddCommand(repoCreditsCmd)
	repoCreditsCmd.Flags().BoolP("static", "s", false, "Print a static version of the credits")
}

var repoCmd = &cobra.Command{
	Use:   "repo <command>",
	Short: "Create, clone, fork, and view repositories",
	Long:  `Work with GitHub repositories`,
	Example: heredoc.Doc(`
	$ gh repo create
	$ gh repo clone cli/cli
	$ gh repo view --web
	`),
	Annotations: map[string]string{
		"IsCore": "true",
		"help:arguments": `
A repository can be supplied as an argument in any of the following formats:
- "OWNER/REPO"
- by URL, e.g. "https://github.com/OWNER/REPO"`},
}

var repoForkCmd = &cobra.Command{
	Use:   "fork [<repository>]",
	Short: "Create a fork of a repository",
	Long: `Create a fork of a repository.

With no argument, creates a fork of the current repository. Otherwise, forks the specified repository.`,
	RunE: repoFork,
}

var repoCreditsCmd = &cobra.Command{
	Use:   "credits [<repository>]",
	Short: "View credits for a repository",
	Example: heredoc.Doc(`
	# view credits for the current repository
	$ gh repo credits
	
	# view credits for a specific repository
	$ gh repo credits cool/repo

	# print a non-animated thank you
	$ gh repo credits -s
	
	# pipe to just print the contributors, one per line
	$ gh repo credits | cat
	`),
	Args:   cobra.MaximumNArgs(1),
	RunE:   repoCredits,
	Hidden: true,
}

var Since = func(t time.Time) time.Duration {
	return time.Since(t)
}

func repoFork(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)

	clonePref, err := cmd.Flags().GetString("clone")
	if err != nil {
		return err
	}
	remotePref, err := cmd.Flags().GetString("remote")
	if err != nil {
		return err
	}

	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return fmt.Errorf("unable to create client: %w", err)
	}

	var repoToFork ghrepo.Interface
	inParent := false // whether or not we're forking the repo we're currently "in"
	if len(args) == 0 {
		baseRepo, err := determineBaseRepo(apiClient, cmd, ctx)
		if err != nil {
			return fmt.Errorf("unable to determine base repository: %w", err)
		}
		inParent = true
		repoToFork = baseRepo
	} else {
		repoArg := args[0]

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

	if !connectedToTerminal(cmd) {
		if (inParent && remotePref == "prompt") || (!inParent && clonePref == "prompt") {
			return errors.New("--remote or --clone must be explicitly set when not attached to tty")
		}
	}

	greenCheck := utils.Green("✓")
	stderr := colorableErr(cmd)
	s := utils.Spinner(stderr)
	stopSpinner := func() {}

	if connectedToTerminal(cmd) {
		loading := utils.Gray("Forking ") + utils.Bold(utils.Gray(ghrepo.FullName(repoToFork))) + utils.Gray("...")
		s.Suffix = " " + loading
		s.FinalMSG = utils.Gray(fmt.Sprintf("- %s\n", loading))
		utils.StartSpinner(s)
		stopSpinner = func() {
			utils.StopSpinner(s)

		}
	}

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
		if connectedToTerminal(cmd) {
			fmt.Fprintf(stderr, "%s %s %s\n",
				utils.Yellow("!"),
				utils.Bold(ghrepo.FullName(forkedRepo)),
				"already exists")
		} else {
			fmt.Fprintf(stderr, "%s already exists", ghrepo.FullName(forkedRepo))
			return nil
		}
	} else {
		if connectedToTerminal(cmd) {
			fmt.Fprintf(stderr, "%s Created fork %s\n", greenCheck, utils.Bold(ghrepo.FullName(forkedRepo)))
		}
	}

	if (inParent && remotePref == "false") || (!inParent && clonePref == "false") {
		return nil
	}

	if inParent {
		remotes, err := ctx.Remotes()
		if err != nil {
			return err
		}
		if remote, err := remotes.FindByRepo(forkedRepo.RepoOwner(), forkedRepo.RepoName()); err == nil {
			if connectedToTerminal(cmd) {
				fmt.Fprintf(stderr, "%s Using existing remote %s\n", greenCheck, utils.Bold(remote.Name))
			}
			return nil
		}

		remoteDesired := remotePref == "true"
		if remotePref == "prompt" {
			err = prompt.Confirm("Would you like to add a remote for the fork?", &remoteDesired)
			if err != nil {
				return fmt.Errorf("failed to prompt: %w", err)
			}
		}
		if remoteDesired {
			remoteName := "origin"

			remotes, err := ctx.Remotes()
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
				if connectedToTerminal(cmd) {
					fmt.Fprintf(stderr, "%s Renamed %s remote to %s\n", greenCheck, utils.Bold(remoteName), utils.Bold(renameTarget))
				}
			}

			forkedRepoCloneURL := formatRemoteURL(cmd, forkedRepo)

			_, err = git.AddRemote(remoteName, forkedRepoCloneURL)
			if err != nil {
				return fmt.Errorf("failed to add remote: %w", err)
			}

			if connectedToTerminal(cmd) {
				fmt.Fprintf(stderr, "%s Added remote %s\n", greenCheck, utils.Bold(remoteName))
			}
		}
	} else {
		cloneDesired := clonePref == "true"
		if clonePref == "prompt" {
			err = prompt.Confirm("Would you like to clone the fork?", &cloneDesired)
			if err != nil {
				return fmt.Errorf("failed to prompt: %w", err)
			}
		}
		if cloneDesired {
			forkedRepoCloneURL := formatRemoteURL(cmd, forkedRepo)
			cloneDir, err := git.RunClone(forkedRepoCloneURL, []string{})
			if err != nil {
				return fmt.Errorf("failed to clone fork: %w", err)
			}

			// TODO This is overly wordy and I'd like to streamline this.
			cfg, err := ctx.Config()
			if err != nil {
				return err
			}
			protocol, err := cfg.Get("", "git_protocol")
			if err != nil {
				return err
			}

			upstreamURL := ghrepo.FormatRemoteURL(repoToFork, protocol)

			err = git.AddUpstreamRemote(upstreamURL, cloneDir)
			if err != nil {
				return err
			}

			if connectedToTerminal(cmd) {
				fmt.Fprintf(stderr, "%s Cloned fork\n", greenCheck)
			}
		}
	}

	return nil
}

func repoCredits(cmd *cobra.Command, args []string) error {
	return credits(cmd, args)
}
