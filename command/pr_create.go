package command

import (
	"fmt"
	"net/url"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/githubtemplate"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func prCreate(cmd *cobra.Command, _ []string) error {
	ctx := contextForCommand(cmd)
	remotes, err := ctx.Remotes()
	if err != nil {
		return err
	}

	client, err := apiClientForContext(ctx)
	if err != nil {
		return fmt.Errorf("could not initialize API client: %w", err)
	}

	baseRepoOverride, _ := cmd.Flags().GetString("repo")
	repoContext, err := context.ResolveRemotesToRepos(remotes, client, baseRepoOverride)
	if err != nil {
		return err
	}

	baseRepo, err := repoContext.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repository: %w", err)
	}

	headBranch, err := ctx.Branch()
	if err != nil {
		return fmt.Errorf("could not determine the current branch: %w", err)
	}

	baseBranch, err := cmd.Flags().GetString("base")
	if err != nil {
		return err
	}
	if baseBranch == "" {
		baseBranch = baseRepo.DefaultBranchRef.Name
	}

	didForkRepo := false
	var headRemote *context.Remote
	headRepo, err := repoContext.HeadRepo()
	if err != nil {
		if baseRepo.IsPrivate {
			return fmt.Errorf("cannot write to private repository '%s'", ghrepo.FullName(baseRepo))
		}
		headRepo, err = api.ForkRepo(client, baseRepo)
		if err != nil {
			return fmt.Errorf("error forking repo: %w", err)
		}
		didForkRepo = true
		// TODO: support non-HTTPS git remote URLs
		baseRepoURL := fmt.Sprintf("https://github.com/%s.git", ghrepo.FullName(baseRepo))
		headRepoURL := fmt.Sprintf("https://github.com/%s.git", ghrepo.FullName(headRepo))
		// TODO: figure out what to name the new git remote
		gitRemote, err := git.AddRemote("fork", baseRepoURL, headRepoURL)
		if err != nil {
			return fmt.Errorf("error adding remote: %w", err)
		}
		headRemote = &context.Remote{
			Remote: gitRemote,
			Owner:  headRepo.RepoOwner(),
			Repo:   headRepo.RepoName(),
		}
	}

	if headBranch == baseBranch && ghrepo.IsSame(baseRepo, headRepo) {
		return fmt.Errorf("must be on a branch named differently than %q", baseBranch)
	}

	if headRemote == nil {
		headRemote, err = repoContext.RemoteForRepo(headRepo)
		if err != nil {
			return fmt.Errorf("git remote not found for head repository: %w", err)
		}
	}

	if ucc, err := git.UncommittedChangeCount(); err == nil && ucc > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", utils.Pluralize(ucc, "uncommitted change"))
	}
	pushTries := 0
	maxPushTries := 3
	for {
		// TODO: respect existing upstream configuration of the current branch
		if err := git.Push(headRemote.Name, fmt.Sprintf("HEAD:%s", headBranch)); err != nil {
			if didForkRepo && pushTries < maxPushTries {
				pushTries++
				// first wait 2 seconds after forking, then 4s, then 6s
				time.Sleep(time.Duration(2*pushTries) * time.Second)
				continue
			}
			return err
		}
		break
	}

	isWeb, err := cmd.Flags().GetBool("web")
	if err != nil {
		return fmt.Errorf("could not parse web: %q", err)
	}
	if isWeb {
		openURL := fmt.Sprintf(`https://github.com/%s/pull/%s`, ghrepo.FullName(headRepo), headBranch)
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", openURL)
		return utils.OpenInBrowser(openURL)
	}

	headBranchLabel := headBranch
	if !ghrepo.IsSame(baseRepo, headRepo) {
		headBranchLabel = fmt.Sprintf("%s:%s", headRepo.RepoOwner(), headBranch)
	}
	fmt.Fprintf(colorableErr(cmd), "\nCreating pull request for %s into %s in %s\n\n",
		utils.Cyan(headBranchLabel),
		utils.Cyan(baseBranch),
		ghrepo.FullName(baseRepo))

	title, err := cmd.Flags().GetString("title")
	if err != nil {
		return fmt.Errorf("could not parse title: %w", err)
	}
	body, err := cmd.Flags().GetString("body")
	if err != nil {
		return fmt.Errorf("could not parse body: %w", err)
	}

	action := SubmitAction

	interactive := title == "" || body == ""

	if interactive {
		var templateFiles []string
		if rootDir, err := git.ToplevelDir(); err == nil {
			// TODO: figure out how to stub this in tests
			templateFiles = githubtemplate.Find(rootDir, "PULL_REQUEST_TEMPLATE")
		}

		tb, err := titleBodySurvey(cmd, title, body, templateFiles)
		if err != nil {
			return fmt.Errorf("could not collect title and/or body: %w", err)
		}

		action = tb.Action

		if action == CancelAction {
			fmt.Fprintln(cmd.ErrOrStderr(), "Discarding.")
			return nil
		}

		if title == "" {
			title = tb.Title
		}
		if body == "" {
			body = tb.Body
		}
	}

	isDraft, err := cmd.Flags().GetBool("draft")
	if err != nil {
		return fmt.Errorf("could not parse draft: %w", err)
	}

	if action == SubmitAction {
		if title == "" {
			return fmt.Errorf("pull request title must not be blank")
		}

		headRefName := headBranch
		if !ghrepo.IsSame(headRemote, baseRepo) {
			headRefName = fmt.Sprintf("%s:%s", headRemote.RepoOwner(), headBranch)
		}

		params := map[string]interface{}{
			"title":       title,
			"body":        body,
			"draft":       isDraft,
			"baseRefName": baseBranch,
			"headRefName": headRefName,
		}

		pr, err := api.CreatePullRequest(client, baseRepo, params)
		if err != nil {
			return fmt.Errorf("failed to create pull request: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), pr.URL)
	} else if action == PreviewAction {
		openURL := fmt.Sprintf(
			"https://github.com/%s/compare/%s...%s?expand=1&title=%s&body=%s",
			ghrepo.FullName(baseRepo),
			baseBranch,
			headBranchLabel,
			url.QueryEscape(title),
			url.QueryEscape(body),
		)
		// TODO could exceed max url length for explorer
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", displayURL(openURL))
		return utils.OpenInBrowser(openURL)
	} else {
		panic("Unreachable state")
	}

	return nil

}

var prCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a pull request",
	RunE:  prCreate,
}

func init() {
	prCreateCmd.Flags().BoolP("draft", "d", false,
		"Mark pull request as a draft")
	prCreateCmd.Flags().StringP("title", "t", "",
		"Supply a title. Will prompt for one otherwise.")
	prCreateCmd.Flags().StringP("body", "b", "",
		"Supply a body. Will prompt for one otherwise.")
	prCreateCmd.Flags().StringP("base", "B", "",
		"The branch into which you want your code merged")
	prCreateCmd.Flags().BoolP("web", "w", false, "Open the web browser to create a pull request")
}
