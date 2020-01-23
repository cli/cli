package command

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

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
	remotes, err := ctx.Remotes()
	if err != nil {
		return err
	}

	client, err := apiClientForContext(ctx)
	if err != nil {
		return errors.Wrap(err, "could not initialize API client")
	}

	baseRepoOverride, _ := cmd.Flags().GetString("repo")
	repoContext, err := resolveRemotesToRepos(remotes, client, baseRepoOverride)
	if err != nil {
		return err
	}

	baseRepo, err := repoContext.BaseRepo()
	if err != nil {
		return errors.Wrap(err, "could not determine the base repository")
	}

	headBranch, err := ctx.Branch()
	if err != nil {
		return errors.Wrap(err, "could not determine the current branch")
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
			return fmt.Errorf("cannot write to private repository '%s/%s'", baseRepo.RepoOwner(), baseRepo.RepoName())
		}
		headRepo, err = api.ForkRepo(client, baseRepo)
		if err != nil {
			return fmt.Errorf("error forking repo: %w", err)
		}
		didForkRepo = true
		// TODO: support non-HTTPS git remote URLs
		baseRepoURL := fmt.Sprintf("https://github.com/%s/%s.git", baseRepo.RepoOwner(), baseRepo.RepoName())
		headRepoURL := fmt.Sprintf("https://github.com/%s/%s.git", headRepo.RepoOwner(), headRepo.RepoName())
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

	if headBranch == baseBranch && isSameRepo(baseRepo, headRepo) {
		return fmt.Errorf("must be on a branch named differently than %q", baseBranch)
	}

	if headRemote == nil {
		headRemote, err = repoContext.RemoteForRepo(headRepo)
		if err != nil {
			return errors.Wrap(err, "git remote not found for head repository")
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
		return errors.Wrap(err, "could not parse web")
	}
	if isWeb {
		openURL := fmt.Sprintf(`https://github.com/%s/%s/pull/%s`, headRepo.RepoOwner(), headRepo.RepoName(), headBranch)
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", openURL)
		return utils.OpenInBrowser(openURL)
	}

	headBranchLabel := headBranch
	if !isSameRepo(baseRepo, headRepo) {
		headBranchLabel = fmt.Sprintf("%s:%s", headRepo.RepoOwner(), headBranch)
	}
	fmt.Fprintf(colorableErr(cmd), "\nCreating pull request for %s into %s in %s/%s\n\n",
		utils.Cyan(headBranchLabel),
		utils.Cyan(baseBranch),
		baseRepo.RepoOwner(),
		baseRepo.RepoName())

	title, err := cmd.Flags().GetString("title")
	if err != nil {
		return errors.Wrap(err, "could not parse title")
	}
	body, err := cmd.Flags().GetString("body")
	if err != nil {
		return errors.Wrap(err, "could not parse body")
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
			return errors.Wrap(err, "could not collect title and/or body")
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
		return errors.Wrap(err, "could not parse draft")
	}

	if action == SubmitAction {
		params := map[string]interface{}{
			"title":       title,
			"body":        body,
			"draft":       isDraft,
			"baseRefName": baseBranch,
			"headRefName": headBranch,
		}

		pr, err := api.CreatePullRequest(client, baseRepo, params)
		if err != nil {
			return errors.Wrap(err, "failed to create pull request")
		}

		fmt.Fprintln(cmd.OutOrStdout(), pr.URL)
	} else if action == PreviewAction {
		openURL := fmt.Sprintf(
			"https://github.com/%s/%s/compare/%s...%s?expand=1&title=%s&body=%s",
			baseRepo.RepoOwner(),
			baseRepo.RepoName(),
			baseBranch,
			headBranchLabel,
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

// cap the number of git remotes looked up, since the user might have an
// unusally large number of git remotes
const maxRemotesForLookup = 5

func resolveRemotesToRepos(remotes context.Remotes, client *api.Client, base string) (resolvedRemotes, error) {
	sort.Stable(remotes)
	lenRemotesForLookup := len(remotes)
	if lenRemotesForLookup > maxRemotesForLookup {
		lenRemotesForLookup = maxRemotesForLookup
	}

	hasBaseOverride := base != ""
	baseOverride := repoFromFullName(base)
	foundBaseOverride := false
	repos := []api.Repo{}
	for _, r := range remotes[:lenRemotesForLookup] {
		repos = append(repos, r)
		if isSameRepo(r, baseOverride) {
			foundBaseOverride = true
		}
	}
	if hasBaseOverride && !foundBaseOverride {
		// additionally, look up the explicitly specified base repo if it's not
		// already covered by git remotes
		repos = append(repos, baseOverride)
	}

	result := resolvedRemotes{remotes: remotes}
	if hasBaseOverride {
		result.baseOverride = baseOverride
	}
	networkResult, err := api.RepoNetwork(client, repos)
	if err != nil {
		return result, err
	}
	result.network = networkResult
	return result, nil
}

type resolvedRemotes struct {
	baseOverride api.Repo
	remotes      context.Remotes
	network      api.RepoNetworkResult
}

// BaseRepo is the first found repository in the "upstream", "github", "origin"
// git remote order, resolved to the parent repo if the git remote points to a fork
func (r resolvedRemotes) BaseRepo() (*api.Repository, error) {
	if r.baseOverride != nil {
		for _, repo := range r.network.Repositories {
			if repo != nil && isSameRepo(repo, r.baseOverride) {
				return repo, nil
			}
		}
		return nil, fmt.Errorf("failed looking up information about the '%s/%s' repository",
			r.baseOverride.RepoOwner(), r.baseOverride.RepoName())
	}

	for _, repo := range r.network.Repositories {
		if repo == nil {
			continue
		}
		if repo.IsFork() {
			return repo.Parent, nil
		}
		return repo, nil
	}

	return nil, errors.New("not found")
}

// HeadRepo is the first found repository that has push access
func (r resolvedRemotes) HeadRepo() (*api.Repository, error) {
	for _, repo := range r.network.Repositories {
		if repo != nil && repo.ViewerCanPush() {
			return repo, nil
		}
	}
	return nil, errors.New("none of the repositories have push access")
}

// RemoteForRepo finds the git remote that points to a repository
func (r resolvedRemotes) RemoteForRepo(repo api.Repo) (*context.Remote, error) {
	for i, remote := range r.remotes {
		if isSameRepo(remote, repo) ||
			// additionally, look up the resolved repository name in case this
			// git remote points to this repository via a redirect
			(r.network.Repositories[i] != nil && isSameRepo(r.network.Repositories[i], repo)) {
			return remote, nil
		}
	}
	return nil, errors.New("not found")
}

type ghRepo struct {
	owner string
	name  string
}

func repoFromFullName(nwo string) (r ghRepo) {
	parts := strings.SplitN(nwo, "/", 2)
	if len(parts) == 2 {
		r.owner, r.name = parts[0], parts[1]
	}
	return
}

func (r ghRepo) RepoOwner() string {
	return r.owner
}
func (r ghRepo) RepoName() string {
	return r.name
}

func isSameRepo(a, b api.Repo) bool {
	return strings.EqualFold(a.RepoOwner(), b.RepoOwner()) &&
		strings.EqualFold(a.RepoName(), b.RepoName())
}
