package command

import (
	"fmt"
	"os"
	"runtime"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/pkg/githubtemplate"
	"github.com/github/gh-cli/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func prCreate(cmd *cobra.Command, _ []string) error {
	ctx := contextForCommand(cmd)

	ucc, err := git.UncommittedChangeCount()
	if err != nil {
		return err
	}
	if ucc > 0 {
		noun := "change"
		if ucc > 1 {
			// TODO: use pluralize helper
			noun = noun + "s"
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %d uncommitted %s\n", ucc, noun)
	}

	repo, err := ctx.BaseRepo()
	if err != nil {
		return errors.Wrap(err, "could not determine GitHub repo")
	}

	head, err := ctx.Branch()
	if err != nil {
		return errors.Wrap(err, "could not determine current branch")
	}

	remote, err := guessRemote(ctx)
	if err != nil {
		return err
	}

	if err = git.Push(remote, fmt.Sprintf("HEAD:%s", head)); err != nil {
		return err
	}

	isWeb, err := cmd.Flags().GetBool("web")
	if err != nil {
		return errors.Wrap(err, "could not parse web")
	}
	if isWeb {
		openURL := fmt.Sprintf(`https://github.com/%s/%s/pull/%s`, repo.RepoOwner(), repo.RepoName(), head)
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", openURL)
		return utils.OpenInBrowser(openURL)
	}

	title, err := cmd.Flags().GetString("title")
	if err != nil {
		return errors.Wrap(err, "could not parse title")
	}
	body, err := cmd.Flags().GetString("body")
	if err != nil {
		return errors.Wrap(err, "could not parse body")
	}

	interactive := title == "" || body == ""

	if interactive {
		var templateFiles []string
		if rootDir, err := git.ToplevelDir(); err == nil {
			// TODO: figure out how to stub this in tests
			templateFiles = githubtemplate.Find(rootDir, "PULL_REQUEST_TEMPLATE")
		}

		tb, err := titleBodySurvey(cmd, title, body, templateFiles)
		if err != nil {
			return errors.Wrap(err, "could not collect title and/or body")
		}

		if tb == nil {
			// editing was canceled, we can just leave
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
		return errors.Wrap(err, "could not parse base")
	}
	if base == "" {
		// TODO: use default branch for the repo
		base = "master"
	}

	client, err := apiClientForContext(ctx)
	if err != nil {
		return errors.Wrap(err, "could not initialize api client")
	}

	isDraft, err := cmd.Flags().GetBool("draft")
	if err != nil {
		return errors.Wrap(err, "could not parse draft")
	}

	params := map[string]interface{}{
		"title":       title,
		"body":        body,
		"draft":       isDraft,
		"baseRefName": base,
		"headRefName": head,
	}

	pr, err := api.CreatePullRequest(client, repo, params)
	if err != nil {
		return errors.Wrap(err, "failed to create pull request")
	}

	fmt.Fprintln(cmd.OutOrStdout(), pr.URL)
	return nil
}

func guessRemote(ctx context.Context) (string, error) {
	remotes, err := ctx.Remotes()
	if err != nil {
		return "", errors.Wrap(err, "could not read git remotes")
	}

	// TODO: consolidate logic with fsContext.BaseRepo
	// TODO: check if the GH repo that the remote points to is writeable
	remote, err := remotes.FindByName("upstream", "github", "origin", "*")
	if err != nil {
		return "", errors.Wrap(err, "could not determine suitable remote")
	}

	return remote.Name, nil
}

func determineEditor() string {
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "nano"
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
