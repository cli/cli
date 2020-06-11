package repo

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/command"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func RepoCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [<name>]",
		Short: "Create a new repository",
		Long:  `Create a new repository`,
		Example: utils.Bold("$ gh repo create") + `
Will create a repository on your account using the name of your current directory

` + utils.Bold("$ gh repo create my-project") + `
Will create a repository on your account using the name 'my-project'

` + utils.Bold("$ gh repo create cli/my-project") + `
Will create a repository in the organization 'cli' using the name 'my-project'`,
		Annotations: map[string]string{"help:arguments": `A repository can be supplied as an argument in any of the following formats:
- <OWNER/REPO>
- by URL, e.g. "https://github.com/OWNER/REPO"`},
		RunE: repoCreate,
	}

	cmd.Flags().StringP("description", "d", "", "Description of repository")
	cmd.Flags().StringP("homepage", "h", "", "Repository home page URL")
	cmd.Flags().StringP("team", "t", "", "The name of the organization team to be granted access")
	cmd.Flags().Bool("enable-issues", true, "Enable issues in the new repository")
	cmd.Flags().Bool("enable-wiki", true, "Enable wiki in the new repository")
	cmd.Flags().Bool("public", false, "Make the new repository public (default: private)")

	return cmd
}

func repoCreate(cmd *cobra.Command, args []string) error {
	var name string
	orgName := ""
	projectDir, projectDirErr := git.ToplevelDir()

	teamSlug, err := cmd.Flags().GetString("team")
	if err != nil {
		return err
	}

	if len(args) > 0 {
		name = args[0]
		if strings.Contains(name, "/") {
			newRepo, err := ghrepo.FromFullName(name)
			if err != nil {
				return fmt.Errorf("argument error: %w", err)
			}
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
		HomepageURL:      homepage,
		HasIssuesEnabled: hasIssuesEnabled,
		HasWikiEnabled:   hasWikiEnabled,
	}

	ctx := command.ContextForCommand(cmd)
	client, err := command.ApiClientForContext(ctx)
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
			// FIXME: duplicates ColorableOut
			out = utils.NewColorable(outFile)
		}
	}

	if isTTY {
		fmt.Fprintf(out, "%s Created repository %s on GitHub\n", greenCheck, ghrepo.FullName(repo))
	} else {
		fmt.Fprintln(out, repo.URL)
	}

	remoteURL := command.FormatRemoteURL(cmd, ghrepo.FullName(repo))

	if projectDirErr == nil {
		_, err = git.AddRemote("origin", remoteURL)
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
			err = run.PrepareCmd(gitInit).Run()
			if err != nil {
				return err
			}
			gitRemoteAdd := git.GitCommand("-C", path, "remote", "add", "origin", remoteURL)
			gitRemoteAdd.Stdout = os.Stdout
			gitRemoteAdd.Stderr = os.Stderr
			err = run.PrepareCmd(gitRemoteAdd).Run()
			if err != nil {
				return err
			}

			fmt.Fprintf(out, "%s Initialized repository in './%s/'\n", greenCheck, path)
		}
	}

	return nil
}
