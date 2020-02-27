package command

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(repoCmd)
	repoCmd.AddCommand(repoCloneCmd)
	repoCmd.AddCommand(repoViewCmd)
	repoCmd.AddCommand(repoForkCmd)

	repoForkCmd.Flags().StringP("clone", "c", "prompt", "true: clone fork. false: never clone fork")
	repoForkCmd.Flags().StringP("remote", "r", "prompt", "true: add remote for fork. false: never add remote fork")
}

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "View repositories",
	Long: `Work with GitHub repositories.

A repository can be supplied as an argument in any of the following formats:
- "OWNER/REPO"
- by URL, e.g. "https://github.com/OWNER/REPO"`,
}

var repoCloneCmd = &cobra.Command{
	Use:   "clone <repo>",
	Args:  cobra.MinimumNArgs(1),
	Short: "Clone a repository locally",
	Long: `Clone a GitHub repository locally.

To pass 'git clone' options, separate them with '--'.`,
	RunE: repoClone,
}

var repoForkCmd = &cobra.Command{
	Use:   "fork [<repository>]",
	Short: "Create a fork of a repository.",
	Long: `Create a fork of a repository.

With no argument, creates a fork of the current repository. Otherwise, forks the specified repository.`,
	RunE: repoFork,
}

var repoViewCmd = &cobra.Command{
	Use:   "view [<repository>]",
	Short: "View a repository in the browser.",
	Long: `View a GitHub repository in the browser.

With no argument, the repository for the current directory is opened.`,
	RunE: repoView,
}

func repoClone(cmd *cobra.Command, args []string) error {
	cloneURL := args[0]
	if !strings.Contains(cloneURL, ":") {
		cloneURL = fmt.Sprintf("https://github.com/%s.git", cloneURL)
	}

	cloneArgs := []string{"clone"}
	cloneArgs = append(cloneArgs, args[1:]...)
	cloneArgs = append(cloneArgs, cloneURL)

	cloneCmd := git.GitCommand(cloneArgs...)
	cloneCmd.Stdin = os.Stdin
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr
	return utils.PrepareCmd(cloneCmd).Run()
}

func isURL(arg string) bool {
	return strings.HasPrefix(arg, "http:/") || strings.HasPrefix(arg, "https:/")
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

	var toFork ghrepo.Interface
	inParent := false // whether or not we're forking the repo we're currently "in"
	if len(args) == 0 {
		baseRepo, err := determineBaseRepo(cmd, ctx)
		if err != nil {
			return fmt.Errorf("unable to determine base repository: %w", err)
		}
		inParent = true
		toFork = baseRepo
	} else {
		repoArg := args[0]

		if isURL(repoArg) {
			parsedURL, err := url.Parse(repoArg)
			if err != nil {
				return fmt.Errorf("did not understand argument: %w", err)
			}

			toFork, err = ghrepo.FromURL(parsedURL)
			if err != nil {
				return fmt.Errorf("did not understand argument: %w", err)
			}

		} else {
			toFork = ghrepo.FromFullName(repoArg)
			if toFork.RepoName() == "" || toFork.RepoOwner() == "" {
				return fmt.Errorf("could not parse owner or repo name from %s", repoArg)
			}
		}
	}

	greenCheck := utils.Green("✓")
	redX := utils.Bold(utils.Red("X"))
	out := colorableOut(cmd)
	s := utils.Spinner()
	loading := utils.Gray("Forking ") + utils.Bold(utils.Gray(ghrepo.FullName(toFork))) + utils.Gray("...")
	s.Suffix = " " + loading
	s.FinalMSG = utils.Gray(fmt.Sprintf("- %s\n", loading))
	s.Start()

	authLogin, err := ctx.AuthLogin()
	if err != nil {
		s.Stop()
		return fmt.Errorf("could not determine current username: %w", err)
	}

	possibleFork := ghrepo.New(authLogin, toFork.RepoName())

	forkedRepo, err := api.ForkRepo(apiClient, toFork)
	if err != nil {
		s.Stop()
		return fmt.Errorf("failed to fork: %w", err)
	}

	// This is weird. There is not an efficient way to determine via the GitHub API whether or not a
	// given user has forked a given repo. We noticed, also, that the create fork API endpoint just
	// returns the fork repo data even if it already exists -- with no change in status code or
	// anything. We thus check the created time to see if the repo is brand new or not; if it's not,
	// we assume the fork already existed and report an error.
	created_ago := Since(forkedRepo.CreatedAt)
	if created_ago > time.Minute {
		s.Stop()
		fmt.Fprintf(out, redX+" ")
		return fmt.Errorf("%s already exists", utils.Bold(ghrepo.FullName(possibleFork)))
	}

	s.Stop()

	fmt.Fprintf(out, "%s Created fork %s\n", greenCheck, utils.Bold(ghrepo.FullName(forkedRepo)))

	if (inParent && remotePref == "false") || (!inParent && clonePref == "false") {
		return nil
	}

	if inParent {
		remoteDesired := remotePref == "true"
		if remotePref == "prompt" {
			err = Confirm("Would you like to add a remote for the new fork?", &remoteDesired)
			if err != nil {
				return fmt.Errorf("failed to prompt: %w", err)
			}
		}
		if remoteDesired {
			_, err := git.AddRemote("fork", forkedRepo.CloneURL, "")
			if err != nil {
				return fmt.Errorf("failed to add remote: %w", err)
			}

			fetchCmd := git.GitCommand("fetch", "fork")
			fetchCmd.Stdin = os.Stdin
			fetchCmd.Stdout = os.Stdout
			fetchCmd.Stderr = os.Stderr
			err = utils.PrepareCmd(fetchCmd).Run()
			if err != nil {
				return fmt.Errorf("failed to fetch new remote: %w", err)
			}

			fmt.Fprintf(out, "%s Remote added at %s\n", greenCheck, utils.Bold("fork"))
		}
	} else {
		cloneDesired := clonePref == "true"
		if clonePref == "prompt" {
			err = Confirm("Would you like to clone the new fork?", &cloneDesired)
			if err != nil {
				return fmt.Errorf("failed to prompt: %w", err)
			}
		}
		if cloneDesired {
			cloneCmd := git.GitCommand("clone", forkedRepo.CloneURL)
			cloneCmd.Stdin = os.Stdin
			cloneCmd.Stdout = os.Stdout
			cloneCmd.Stderr = os.Stderr
			err = utils.PrepareCmd(cloneCmd).Run()
			if err != nil {
				return fmt.Errorf("failed to clone fork: %w", err)
			}

			fmt.Fprintf(out, "%s Cloned fork\n", greenCheck)
		}
	}

	return nil
}

var Confirm = func(prompt string, result *bool) error {
	p := &survey.Confirm{
		Message: prompt,
		Default: true,
	}
	return survey.AskOne(p, result)
}

func repoView(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)

	var openURL string
	if len(args) == 0 {
		baseRepo, err := determineBaseRepo(cmd, ctx)
		if err != nil {
			return err
		}
		openURL = fmt.Sprintf("https://github.com/%s", ghrepo.FullName(baseRepo))
	} else {
		repoArg := args[0]
		if isURL(repoArg) {
			openURL = repoArg
		} else {
			openURL = fmt.Sprintf("https://github.com/%s", repoArg)
		}
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", displayURL(openURL))
	return utils.OpenInBrowser(openURL)
}
