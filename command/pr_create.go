package command

import (
	"errors"
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

type defaults struct {
	Title string
	Body  string
}

func computeDefaults(baseRef, headRef string) (defaults, error) {
	commits, err := git.Commits(baseRef, headRef)
	if err != nil {
		return defaults{}, err
	}

	out := defaults{}

	if len(commits) == 1 {
		out.Title = commits[0].Title
		body, err := git.CommitBody(commits[0].Sha)
		if err != nil {
			return defaults{}, err
		}
		out.Body = body
	} else {
		out.Title = headRef // TODO format or something?

		body := ""
		for _, c := range commits {
			body += fmt.Sprintf("- %s\n", c.Title)
		}
		out.Body = body
	}

	return out, nil
}

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
	headRepo, headRepoErr := repoContext.HeadRepo()

	baseBranch, err := cmd.Flags().GetString("base")
	if err != nil {
		return err
	}
	if baseBranch == "" {
		baseBranch = baseRepo.DefaultBranchRef.Name
	}
	if headBranch == baseBranch && headRepo != nil && ghrepo.IsSame(baseRepo, headRepo) {
		return fmt.Errorf("must be on a branch named differently than %q", baseBranch)
	}

	if ucc, err := git.UncommittedChangeCount(); err == nil && ucc > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", utils.Pluralize(ucc, "uncommitted change"))
	}

	title, err := cmd.Flags().GetString("title")
	if err != nil {
		return fmt.Errorf("could not parse title: %w", err)
	}
	body, err := cmd.Flags().GetString("body")
	if err != nil {
		return fmt.Errorf("could not parse body: %w", err)
	}

	defs, defaultsErr := computeDefaults(baseBranch, headBranch)

	isWeb, err := cmd.Flags().GetBool("web")
	if err != nil {
		return fmt.Errorf("could not parse web: %q", err)
	}

	autofill, err := cmd.Flags().GetBool("fill")
	if err != nil {
		return fmt.Errorf("could not parse fill: %q", err)
	}

	action := SubmitAction
	if isWeb {
		action = PreviewAction
		if (title == "" || body == "") && defaultsErr != nil {
			return fmt.Errorf("could not compute title or body defaults: %w", defaultsErr)
		}
	} else if autofill {
		if defaultsErr != nil {
			return fmt.Errorf("could not compute title or body defaults: %w", defaultsErr)
		}
		action = SubmitAction
		title = defs.Title
		body = defs.Body
	} else {
		fmt.Fprintf(colorableErr(cmd), "\nCreating pull request for %s into %s in %s\n\n",
			utils.Cyan(headBranch),
			utils.Cyan(baseBranch),
			ghrepo.FullName(baseRepo))
		if (title == "" || body == "") && defaultsErr != nil {
			fmt.Fprintf(colorableErr(cmd), "%s warning: could not compute title or body defaults: %s\n", utils.Yellow("!"), defaultsErr)
		}
	}

	// TODO: only drop into interactive mode if stdin & stdout are a tty
	if !isWeb && !autofill && (title == "" || body == "") {
		var templateFiles []string
		if rootDir, err := git.ToplevelDir(); err == nil {
			// TODO: figure out how to stub this in tests
			templateFiles = githubtemplate.Find(rootDir, "PULL_REQUEST_TEMPLATE")
		}

		tb, err := titleBodySurvey(cmd, title, body, defs, templateFiles)
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

	if action == SubmitAction && title == "" {
		return errors.New("pull request title must not be blank")
	}

	isDraft, err := cmd.Flags().GetBool("draft")
	if err != nil {
		return fmt.Errorf("could not parse draft: %w", err)
	}
	if isDraft && isWeb {
		return errors.New("the --draft flag is not supported with --web")
	}

	didForkRepo := false
	var headRemote *context.Remote
	if headRepoErr != nil {
		if baseRepo.IsPrivate {
			return fmt.Errorf("cannot fork private repository '%s'", ghrepo.FullName(baseRepo))
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

	headBranchLabel := headBranch
	if !ghrepo.IsSame(baseRepo, headRepo) {
		headBranchLabel = fmt.Sprintf("%s:%s", headRepo.RepoOwner(), headBranch)
	}

	if headRemote == nil {
		headRemote, err = repoContext.RemoteForRepo(headRepo)
		if err != nil {
			return fmt.Errorf("git remote not found for head repository: %w", err)
		}
	}

	pushTries := 0
	maxPushTries := 3
	for {
		// TODO: respect existing upstream configuration of the current branch
		if err := git.Push(headRemote.Name, fmt.Sprintf("HEAD:%s", headBranch)); err != nil {
			if didForkRepo && pushTries < maxPushTries {
				pushTries++
				// first wait 2 seconds after forking, then 4s, then 6s
				waitSeconds := 2 * pushTries
				fmt.Fprintf(cmd.ErrOrStderr(), "waiting %s before retrying...\n", utils.Pluralize(waitSeconds, "second"))
				time.Sleep(time.Duration(waitSeconds) * time.Second)
				continue
			}
			return err
		}
		break
	}

	if action == SubmitAction {
		params := map[string]interface{}{
			"title":       title,
			"body":        body,
			"draft":       isDraft,
			"baseRefName": baseBranch,
			"headRefName": headBranchLabel,
		}

		pr, err := api.CreatePullRequest(client, baseRepo, params)
		if err != nil {
			return fmt.Errorf("failed to create pull request: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), pr.URL)
	} else if action == PreviewAction {
		openURL := generateCompareURL(baseRepo, baseBranch, headBranchLabel, title, body)
		// TODO could exceed max url length for explorer
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", displayURL(openURL))
		return utils.OpenInBrowser(openURL)
	} else {
		panic("Unreachable state")
	}

	return nil
}

func generateCompareURL(r ghrepo.Interface, base, head, title, body string) string {
	u := fmt.Sprintf(
		"https://github.com/%s/compare/%s...%s?expand=1",
		ghrepo.FullName(r),
		base,
		head,
	)
	if title != "" {
		u += "&title=" + url.QueryEscape(title)
	}
	if body != "" {
		u += "&body=" + url.QueryEscape(body)
	}
	return u
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
	prCreateCmd.Flags().BoolP("fill", "f", false, "Do not prompt for title/body and just use commit info")
}
