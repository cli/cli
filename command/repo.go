package command

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(repoCmd)
	repoCmd.AddCommand(repoCloneCmd)

	repoCmd.AddCommand(repoCreateCmd)
	repoCreateCmd.Flags().StringP("description", "d", "", "Description of repository")
	repoCreateCmd.Flags().StringP("homepage", "h", "", "Repository home page URL")
	repoCreateCmd.Flags().StringP("team", "t", "", "The name of the organization team to be granted access")
	repoCreateCmd.Flags().Bool("enable-issues", true, "Enable issues in the new repository")
	repoCreateCmd.Flags().Bool("enable-wiki", true, "Enable wiki in the new repository")
	repoCreateCmd.Flags().Bool("public", false, "Make the new repository public")

	repoCmd.AddCommand(repoForkCmd)
	repoForkCmd.Flags().String("clone", "prompt", "Clone fork: {true|false|prompt}")
	repoForkCmd.Flags().String("remote", "prompt", "Add remote for fork: {true|false|prompt}")
	repoForkCmd.Flags().Lookup("clone").NoOptDefVal = "true"
	repoForkCmd.Flags().Lookup("remote").NoOptDefVal = "true"

	repoCmd.AddCommand(repoViewCmd)
}

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Create, clone, fork, and view repositories",
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

var repoCreateCmd = &cobra.Command{
	Use:   "create [<name>]",
	Short: "Create a new repository",
	Long: `Create a new GitHub repository.

Use the "ORG/NAME" syntax to create a repository within your organization.`,
	RunE: repoCreate,
}

var repoForkCmd = &cobra.Command{
	Use:   "fork [<repository>]",
	Short: "Create a fork of a repository",
	Long: `Create a fork of a repository.

With no argument, creates a fork of the current repository. Otherwise, forks the specified repository.`,
	RunE: repoFork,
}

var repoViewCmd = &cobra.Command{
	Use:   "view [<repository>]",
	Short: "View a repository in the browser",
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

func repoCreate(cmd *cobra.Command, args []string) error {
	projectDir, projectDirErr := git.ToplevelDir()

	orgName := ""
	teamSlug, err := cmd.Flags().GetString("team")
	if err != nil {
		return err
	}

	var name string
	if len(args) > 0 {
		name = args[0]
		if strings.Contains(name, "/") {
			newRepo := ghrepo.FromFullName(name)
			orgName = newRepo.RepoOwner()
			name = newRepo.RepoName()
		}
	} else {
		if projectDirErr != nil {
			return projectDirErr
		}
		name = path.Base(projectDir)
	}

	isPublic, err := cmd.Flags().GetBool("public")
	if err != nil {
		return err
	}
	hasIssuesEnabled, err := cmd.Flags().GetBool("enable-issues")
	if err != nil {
		return err
	}
	hasWikiEnabled, err := cmd.Flags().GetBool("enable-wiki")
	if err != nil {
		return err
	}
	description, err := cmd.Flags().GetString("description")
	if err != nil {
		return err
	}
	homepage, err := cmd.Flags().GetString("homepage")
	if err != nil {
		return err
	}

	// TODO: move this into constant within `api`
	visibility := "PRIVATE"
	if isPublic {
		visibility = "PUBLIC"
	}

	input := api.RepoCreateInput{
		Name:             name,
		Visibility:       visibility,
		OwnerID:          orgName,
		TeamID:           teamSlug,
		Description:      description,
		Homepage:         homepage,
		HasIssuesEnabled: hasIssuesEnabled,
		HasWikiEnabled:   hasWikiEnabled,
	}

	ctx := contextForCommand(cmd)
	client, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	repo, err := api.RepoCreate(client, input)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	greenCheck := utils.Green("✓")
	isTTY := false
	if outFile, isFile := out.(*os.File); isFile {
		isTTY = utils.IsTerminal(outFile)
		if isTTY {
			// FIXME: duplicates colorableOut
			out = utils.NewColorable(outFile)
		}
	}

	if isTTY {
		fmt.Fprintf(out, "%s Created repository %s on GitHub\n", greenCheck, ghrepo.FullName(repo))
	} else {
		fmt.Fprintln(out, repo.URL)
	}

	remoteURL := repo.URL + ".git"

	if projectDirErr == nil {
		// TODO: use git.AddRemote
		remoteAdd := git.GitCommand("remote", "add", "origin", remoteURL)
		remoteAdd.Stdout = os.Stdout
		remoteAdd.Stderr = os.Stderr
		err = utils.PrepareCmd(remoteAdd).Run()
		if err != nil {
			return err
		}
		if isTTY {
			fmt.Fprintf(out, "%s Added remote %s\n", greenCheck, remoteURL)
		}
	} else if isTTY {
		doSetup := false
		err := Confirm(fmt.Sprintf("Create a local project directory for %s?", ghrepo.FullName(repo)), &doSetup)
		if err != nil {
			return err
		}

		if doSetup {
			path := repo.Name

			gitInit := git.GitCommand("init", path)
			gitInit.Stdout = os.Stdout
			gitInit.Stderr = os.Stderr
			err = utils.PrepareCmd(gitInit).Run()
			if err != nil {
				return err
			}
			gitRemoteAdd := git.GitCommand("-C", path, "remote", "add", "origin", remoteURL)
			gitRemoteAdd.Stdout = os.Stdout
			gitRemoteAdd.Stderr = os.Stderr
			err = utils.PrepareCmd(gitRemoteAdd).Run()
			if err != nil {
				return err
			}

			fmt.Fprintf(out, "%s Initialized repository in './%s/'\n", greenCheck, path)
		}
	}

	return nil
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

	s.Stop()
	// This is weird. There is not an efficient way to determine via the GitHub API whether or not a
	// given user has forked a given repo. We noticed, also, that the create fork API endpoint just
	// returns the fork repo data even if it already exists -- with no change in status code or
	// anything. We thus check the created time to see if the repo is brand new or not; if it's not,
	// we assume the fork already existed and report an error.
	created_ago := Since(forkedRepo.CreatedAt)
	if created_ago > time.Minute {
		fmt.Fprintf(out, "%s %s %s\n",
			utils.Yellow("!"),
			utils.Bold(ghrepo.FullName(possibleFork)),
			"already exists")
	} else {
		fmt.Fprintf(out, "%s Created fork %s\n", greenCheck, utils.Bold(ghrepo.FullName(forkedRepo)))
	}

	if (inParent && remotePref == "false") || (!inParent && clonePref == "false") {
		return nil
	}

	if inParent {
		remoteDesired := remotePref == "true"
		if remotePref == "prompt" {
			err = Confirm("Would you like to add a remote for the fork?", &remoteDesired)
			if err != nil {
				return fmt.Errorf("failed to prompt: %w", err)
			}
		}
		if remoteDesired {
			_, err := git.AddRemote("fork", forkedRepo.CloneURL, "")
			if err != nil {
				return fmt.Errorf("failed to add remote: %w", err)
			}

			fmt.Fprintf(out, "%s Remote added at %s\n", greenCheck, utils.Bold("fork"))
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
	var toView ghrepo.Interface
	if len(args) == 0 {
		var err error
		toView, err = determineBaseRepo(cmd, ctx)
		if err != nil {
			return err
		}
	} else {
		repoArg := args[0]
		if isURL(repoArg) {
			parsedURL, err := url.Parse(repoArg)
			if err != nil {
				return fmt.Errorf("did not understand argument: %w", err)
			}

			toView, err = ghrepo.FromURL(parsedURL)
			if err != nil {
				return fmt.Errorf("did not understand argument: %w", err)
			}
		} else {
			toView = ghrepo.FromFullName(repoArg)
		}
	}

	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}
	_, err = api.GitHubRepo(apiClient, toView)
	if err != nil {
		return err
	}

	openURL := fmt.Sprintf("https://github.com/%s", ghrepo.FullName(toView))
	fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", displayURL(openURL))
	return utils.OpenInBrowser(openURL)
}
