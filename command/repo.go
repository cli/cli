package command

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"text/template"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
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
	repoViewCmd.Flags().BoolP("web", "w", false, "Open a repository in the browser")
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
	Use:   "clone <repository> [<directory>]",
	Args:  cobra.MinimumNArgs(1),
	Short: "Clone a repository locally",
	Long: `Clone a GitHub repository locally.

To pass 'git clone' flags, separate them with '--'.`,
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
	Short: "View a repository",
	Long: `Display the description and the README of a GitHub repository.

With no argument, the repository for the current directory is displayed.

With '--web', open the repository in a web browser instead.`,
	RunE: repoView,
}

func parseCloneArgs(extraArgs []string) (args []string, target string) {
	args = extraArgs

	if len(args) > 0 {
		if !strings.HasPrefix(args[0], "-") {
			target, args = args[0], args[1:]
		}
	}
	return
}

func runClone(cloneURL string, args []string) (target string, err error) {
	cloneArgs, target := parseCloneArgs(args)

	cloneArgs = append(cloneArgs, cloneURL)

	// If the args contain an explicit target, pass it to clone
	//    otherwise, parse the URL to determine where git cloned it to so we can return it
	if target != "" {
		cloneArgs = append(cloneArgs, target)
	} else {
		target = path.Base(strings.TrimSuffix(cloneURL, ".git"))
	}

	cloneArgs = append([]string{"clone"}, cloneArgs...)

	cloneCmd := git.GitCommand(cloneArgs...)
	cloneCmd.Stdin = os.Stdin
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr

	err = run.PrepareCmd(cloneCmd).Run()
	return
}

func repoClone(cmd *cobra.Command, args []string) error {
	cloneURL := args[0]
	if !strings.Contains(cloneURL, ":") {
		cloneURL = formatRemoteURL(cmd, cloneURL)
	}

	var repo ghrepo.Interface
	var parentRepo ghrepo.Interface

	// TODO: consider caching and reusing `git.ParseSSHConfig().Translator()`
	// here to handle hostname aliases in SSH remotes
	if u, err := git.ParseURL(cloneURL); err == nil {
		repo, _ = ghrepo.FromURL(u)
	}

	if repo != nil {
		ctx := contextForCommand(cmd)
		apiClient, err := apiClientForContext(ctx)
		if err != nil {
			return err
		}

		parentRepo, err = api.RepoParent(apiClient, repo)
		if err != nil {
			return err
		}
	}

	cloneDir, err := runClone(cloneURL, args[1:])
	if err != nil {
		return err
	}

	if parentRepo != nil {
		err := addUpstreamRemote(cmd, parentRepo, cloneDir)
		if err != nil {
			return err
		}
	}

	return nil
}

func addUpstreamRemote(cmd *cobra.Command, parentRepo ghrepo.Interface, cloneDir string) error {
	upstreamURL := formatRemoteURL(cmd, ghrepo.FullName(parentRepo))

	cloneCmd := git.GitCommand("-C", cloneDir, "remote", "add", "-f", "upstream", upstreamURL)
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr
	return run.PrepareCmd(cloneCmd).Run()
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
		HomepageURL:      homepage,
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

	remoteURL := formatRemoteURL(cmd, ghrepo.FullName(repo))

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

	var repoToFork ghrepo.Interface
	inParent := false // whether or not we're forking the repo we're currently "in"
	if len(args) == 0 {
		baseRepo, err := determineBaseRepo(cmd, ctx)
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
			repoToFork = ghrepo.FromFullName(repoArg)
			if repoToFork.RepoName() == "" || repoToFork.RepoOwner() == "" {
				return fmt.Errorf("could not parse owner or repo name from %s", repoArg)
			}
		}
	}

	greenCheck := utils.Green("✓")
	out := colorableOut(cmd)
	s := utils.Spinner(out)
	loading := utils.Gray("Forking ") + utils.Bold(utils.Gray(ghrepo.FullName(repoToFork))) + utils.Gray("...")
	s.Suffix = " " + loading
	s.FinalMSG = utils.Gray(fmt.Sprintf("- %s\n", loading))
	s.Start()

	forkedRepo, err := api.ForkRepo(apiClient, repoToFork)
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

			forkedRepoCloneURL := formatRemoteURL(cmd, ghrepo.FullName(forkedRepo))

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
			forkedRepoCloneURL := formatRemoteURL(cmd, ghrepo.FullName(forkedRepo))
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
	repo, err := api.GitHubRepo(apiClient, toView)
	if err != nil {
		return err
	}

	web, err := cmd.Flags().GetBool("web")
	if err != nil {
		return err
	}

	fullName := ghrepo.FullName(toView)

	openURL := fmt.Sprintf("https://github.com/%s", fullName)
	if web {
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", displayURL(openURL))
		return utils.OpenInBrowser(openURL)
	}

	repoTmpl := `
{{.FullName}}
{{.Description}}

{{.Readme}}

{{.View}}
`

	tmpl, err := template.New("repo").Parse(repoTmpl)
	if err != nil {
		return err
	}

	readmeContent, _ := api.RepositoryReadme(apiClient, fullName)

	if readmeContent == "" {
		readmeContent = utils.Gray("No README provided")
	}

	description := repo.Description
	if description == "" {
		description = utils.Gray("No description provided")
	}

	repoData := struct {
		FullName    string
		Description string
		Readme      string
		View        string
	}{
		FullName:    utils.Bold(fullName),
		Description: description,
		Readme:      readmeContent,
		View:        utils.Gray(fmt.Sprintf("View this repository on GitHub: %s", openURL)),
	}

	out := colorableOut(cmd)

	err = tmpl.Execute(out, repoData)
	if err != nil {
		return err
	}

	return nil
}
