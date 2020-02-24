package command

import (
	"fmt"
	"net/url"
	"os"
	"strings"

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

	repoForkCmd.Flags().BoolP("yes", "y", false, "Run non-interactively, saying yes to prompts")
	repoForkCmd.Flags().BoolP("no", "n", false, "Run non-interactively, saying no to prompts")
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

func repoFork(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)

	forceYes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}
	forceNo, err := cmd.Flags().GetBool("no")
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

	out := colorableOut(cmd)
	fmt.Fprintf(out, "Forking %s...\n", utils.Cyan(ghrepo.FullName(toFork)))

	authLogin, err := ctx.AuthLogin()
	if err != nil {
		return fmt.Errorf("could not determine current username: %w", err)
	}

	possibleFork := ghrepo.New(authLogin, toFork.RepoName())
	exists, err := api.RepoExistsOnGitHub(apiClient, possibleFork)
	if err != nil {
		return fmt.Errorf("problem with API request: %w", err)
	}

	if exists {
		return fmt.Errorf("%s %s", utils.Cyan(ghrepo.FullName(possibleFork)), utils.Red("already exists!"))
	}

	forkedRepo, err := api.ForkRepo(apiClient, toFork)
	if err != nil {
		return fmt.Errorf("failed to fork: %w", err)
	}

	fmt.Fprintf(out, "%s %s %s!\n",
		utils.Cyan(ghrepo.FullName(toFork)),
		utils.Green("successfully forked to"),
		utils.Cyan(ghrepo.FullName(forkedRepo)))

	if forceNo {
		return nil
	}

	if inParent {
		if !forceYes {
			remoteDesired := forceYes
			if !forceYes {
				prompt := &survey.Confirm{
					Message: "Would you like to add a remote for the new fork?",
				}
				err = survey.AskOne(prompt, &remoteDesired)
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

				fmt.Fprintf(out, "%s %s\n", utils.Green("remote added at "), utils.Cyan("fork"))
			}
		}
	} else {
		cloneDesired := forceYes
		if !forceYes {
			prompt := &survey.Confirm{
				Message: "Would you like to clone the new fork?",
			}
			err = survey.AskOne(prompt, &cloneDesired)
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
		}
	}

	return nil
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
