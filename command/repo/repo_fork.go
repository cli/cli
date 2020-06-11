package repo

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/command"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func RepoForkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fork [<repository>]",
		Short: "Create a fork of a repository",
		Long: `Create a fork of a repository.

With no argument, creates a fork of the current repository. Otherwise, forks the specified repository.`,
		RunE: repoFork,
	}

	cmd.Flags().String("clone", "prompt", "Clone fork: {true|false|prompt}")
	cmd.Flags().String("remote", "prompt", "Add remote for fork: {true|false|prompt}")
	cmd.Flags().Lookup("clone").NoOptDefVal = "true"
	cmd.Flags().Lookup("remote").NoOptDefVal = "true"

	return cmd
}

func repoFork(cmd *cobra.Command, args []string) error {
	ctx := command.ContextForCommand(cmd)

	clonePref, err := cmd.Flags().GetString("clone")
	if err != nil {
		return err
	}
	remotePref, err := cmd.Flags().GetString("remote")
	if err != nil {
		return err
	}

	apiClient, err := command.ApiClientForContext(ctx)
	if err != nil {
		return fmt.Errorf("unable to create client: %w", err)
	}

	var repoToFork ghrepo.Interface
	inParent := false // whether or not we're forking the repo we're currently "in"
	if len(args) == 0 {
		baseRepo, err := command.DetermineBaseRepo(apiClient, cmd, ctx)
		if err != nil {
			return fmt.Errorf("unable to determine base repository: %w", err)
		}
		inParent = true
		repoToFork = baseRepo
	} else {
		repoArg := args[0]

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

	greenCheck := utils.Green("✓")
	out := command.ColorableOut(cmd)
	s := utils.Spinner(out)
	loading := utils.Gray("Forking ") + utils.Bold(utils.Gray(ghrepo.FullName(repoToFork))) + utils.Gray("...")
	s.Suffix = " " + loading
	s.FinalMSG = utils.Gray(fmt.Sprintf("- %s\n", loading))
	utils.StartSpinner(s)

	forkedRepo, err := api.ForkRepo(apiClient, repoToFork)
	if err != nil {
		utils.StopSpinner(s)
		return fmt.Errorf("failed to fork: %w", err)
	}

	s.Stop()
	// This is weird. There is not an efficient way to determine via the GitHub API whether or not a
	// given user has forked a given repo. We noticed, also, that the create fork API endpoint just
	// returns the fork repo data even if it already exists -- with no change in status code or
	// anything. We thus check the created time to see if the repo is brand new or not; if it's not,
	// we assume the fork already existed and report an error.
	createdAgo := Since(forkedRepo.CreatedAt)
	if createdAgo > time.Minute {
		fmt.Fprintf(out, "%s %s %s\n",
			utils.Yellow("!"),
			utils.Bold(ghrepo.FullName(forkedRepo)),
			"already exists")
	} else {
		fmt.Fprintf(out, "%s Created fork %s\n", greenCheck, utils.Bold(ghrepo.FullName(forkedRepo)))
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
			fmt.Fprintf(out, "%s Using existing remote %s\n", greenCheck, utils.Bold(remote.Name))
			return nil
		}

		remoteDesired := remotePref == "true"
		if remotePref == "prompt" {
			err = Confirm("Would you like to add a remote for the fork?", &remoteDesired)
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
				fmt.Fprintf(out, "%s Renamed %s remote to %s\n", greenCheck, utils.Bold(remoteName), utils.Bold(renameTarget))
			}

			forkedRepoCloneURL := command.FormatRemoteURL(cmd, ghrepo.FullName(forkedRepo))

			_, err = git.AddRemote(remoteName, forkedRepoCloneURL)
			if err != nil {
				return fmt.Errorf("failed to add remote: %w", err)
			}

			fmt.Fprintf(out, "%s Added remote %s\n", greenCheck, utils.Bold(remoteName))
		}
	} else {
		cloneDesired := clonePref == "true"
		if clonePref == "prompt" {
			err = Confirm("Would you like to clone the fork?", &cloneDesired)
			if err != nil {
				return fmt.Errorf("failed to prompt: %w", err)
			}
		}
		if cloneDesired {
			forkedRepoCloneURL := command.FormatRemoteURL(cmd, ghrepo.FullName(forkedRepo))
			cloneDir, err := runClone(forkedRepoCloneURL, []string{})
			if err != nil {
				return fmt.Errorf("failed to clone fork: %w", err)
			}

			err = addUpstreamRemote(cmd, repoToFork, cloneDir)
			if err != nil {
				return err
			}

			fmt.Fprintf(out, "%s Cloned fork\n", greenCheck)
		}
	}

	return nil
}
