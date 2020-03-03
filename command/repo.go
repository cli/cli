package command

import (
	"fmt"
	"os"
	"path"
	"strings"

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
	repoCmd.AddCommand(repoViewCmd)
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

var repoCreateCmd = &cobra.Command{
	Use:   "create [<name>]",
	Short: "Create a new repository",
	Long: `Create a new GitHub repository.

Use the "ORG/NAME" syntax to create a repository within your organization.`,
	RunE: repoCreate,
}

var repoViewCmd = &cobra.Command{
	Use:   "view [<repo>]",
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
		// TODO: use overridable Confirm
		err := survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("Create a local project directory for %s?", ghrepo.FullName(repo)),
			Default: true,
		}, &doSetup)
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
		if strings.HasPrefix(repoArg, "http:/") || strings.HasPrefix(repoArg, "https:/") {
			openURL = repoArg
		} else {
			openURL = fmt.Sprintf("https://github.com/%s", repoArg)
		}
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", displayURL(openURL))
	return utils.OpenInBrowser(openURL)
}
