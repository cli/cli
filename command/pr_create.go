package command

import (
	"fmt"
	"net/url"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/pkg/githubtemplate"
	"github.com/github/gh-cli/utils"
	"github.com/spf13/cobra"
)

func prCreate(cmd *cobra.Command, _ []string) error {
	ctx := contextForCommand(cmd)

	ucc, err := git.UncommittedChangeCount()
	if err != nil {
		return err
	}
	if ucc > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", utils.Pluralize(ucc, "uncommitted change"))
	}

	repo, err := ctx.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine GitHub repo: %w", err)
	}

	head, err := ctx.Branch()
	if err != nil {
		return fmt.Errorf("could not determine current branch: %w", err)
	}

	remote, err := guessRemote(ctx)
	if err != nil {
		return err
	}

	target, err := cmd.Flags().GetString("base")
	if err != nil {
		return err
	}
	if target == "" {
		// TODO use default branch
		target = "master"
	}

	fmt.Fprintf(colorableErr(cmd), "\nCreating pull request for %s into %s in %s/%s\n\n", utils.Cyan(head), utils.Cyan(target), repo.RepoOwner(), repo.RepoName())

	if err = git.Push(remote, fmt.Sprintf("HEAD:%s", head)); err != nil {
		return err
	}

	isWeb, err := cmd.Flags().GetBool("web")
	if err != nil {
		return fmt.Errorf("could not parse web: %q", err)
	}
	if isWeb {
		openURL := fmt.Sprintf(`https://github.com/%s/%s/pull/%s`, repo.RepoOwner(), repo.RepoName(), head)
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", openURL)
		return utils.OpenInBrowser(openURL)
	}

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

	base, err := cmd.Flags().GetString("base")
	if err != nil {
		return fmt.Errorf("could not parse base: %w", err)
	}
	if base == "" {
		// TODO: use default branch for the repo
		base = "master"
	}

	client, err := apiClientForContext(ctx)
	if err != nil {
		return fmt.Errorf("could not initialize API client: %w", err)
	}

	isDraft, err := cmd.Flags().GetBool("draft")
	if err != nil {
		return fmt.Errorf("could not parse draft: %w", err)
	}

	if action == SubmitAction {
		params := map[string]interface{}{
			"title":       title,
			"body":        body,
			"draft":       isDraft,
			"baseRefName": base,
			"headRefName": head,
		}

		pr, err := api.CreatePullRequest(client, repo, params)
		if err != nil {
			return fmt.Errorf("failed to create pull request: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), pr.URL)
	} else if action == PreviewAction {
		openURL := fmt.Sprintf(
			"https://github.com/%s/%s/compare/%s...%s?expand=1&title=%s&body=%s",
			repo.RepoOwner(),
			repo.RepoName(),
			target,
			head,
			url.QueryEscape(title),
			url.QueryEscape(body),
		)
		// TODO could exceed max url length for explorer
		url, err := url.Parse(openURL)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s%s in your browser.\n", url.Host, url.Path)
		return utils.OpenInBrowser(openURL)
	} else {
		panic("Unreachable state")
	}

	return nil

}

func guessRemote(ctx context.Context) (string, error) {
	remotes, err := ctx.Remotes()
	if err != nil {
		return "", fmt.Errorf("could not read git remotes: %w", err)
	}

	// TODO: consolidate logic with fsContext.BaseRepo
	// TODO: check if the GH repo that the remote points to is writeable
	remote, err := remotes.FindByName("upstream", "github", "origin", "*")
	if err != nil {
		return "", fmt.Errorf("could not determine suitable remote: %w", err)
	}

	return remote.Name, nil
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
