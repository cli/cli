package shared

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/set"
)

type PRFinder interface {
	Find(opts FindOptions) (*api.PullRequest, ghrepo.Interface, error)
}

type progressIndicator interface {
	StartProgressIndicator()
	StopProgressIndicator()
}

type finder struct {
	baseRepoFn func() (ghrepo.Interface, error)
	branchFn   func() (string, error)
	remotesFn  func() (context.Remotes, error)
	httpClient func() (*http.Client, error)
	progress   progressIndicator

	repo       ghrepo.Interface
	prNumber   int
	branchName string
}

func NewFinder(factory *cmdutil.Factory) PRFinder {
	if runCommandFinder != nil {
		f := runCommandFinder
		runCommandFinder = &mockFinder{err: errors.New("you must use a RunCommandFinder to stub PR lookups")}
		return f
	}

	return &finder{
		baseRepoFn: factory.BaseRepo,
		branchFn:   factory.Branch,
		remotesFn:  factory.Remotes,
		httpClient: factory.HttpClient,
		progress:   factory.IOStreams,
	}
}

var runCommandFinder PRFinder

// RunCommandFinder is the NewMockFinder substitute to be used ONLY in runCommand-style tests.
func RunCommandFinder(selector string, pr *api.PullRequest, repo ghrepo.Interface) {
	runCommandFinder = NewMockFinder(selector, pr, repo)
}

type FindOptions struct {
	// Selector can be a number with optional `#` prefix, a branch name with optional `<owner>:` prefix, or
	// a PR URL.
	Selector string
	// Fields lists the GraphQL fields to fetch for the PullRequest.
	Fields []string
	// BaseBranch is the name of the base branch to scope the PR-for-branch lookup to.
	BaseBranch string
	// States lists the possible PR states to scope the PR-for-branch lookup to.
	States []string
}

func (f *finder) Find(opts FindOptions) (*api.PullRequest, ghrepo.Interface, error) {
	if len(opts.Fields) == 0 {
		return nil, nil, errors.New("Find error: no fields specified")
	}

	_ = f.parseURL(opts.Selector)

	if f.repo == nil {
		repo, err := f.baseRepoFn()
		if err != nil {
			return nil, nil, fmt.Errorf("could not determine base repo: %w", err)
		}
		f.repo = repo
	}

	if opts.Selector == "" {
		if err := f.parseCurrentBranch(); err != nil {
			return nil, nil, err
		}
	} else if f.prNumber == 0 {
		if prNumber, err := strconv.Atoi(strings.TrimPrefix(opts.Selector, "#")); err == nil {
			f.prNumber = prNumber
		} else {
			f.branchName = opts.Selector
		}
	}

	httpClient, err := f.httpClient()
	if err != nil {
		return nil, nil, err
	}

	if f.progress != nil {
		f.progress.StartProgressIndicator()
		defer f.progress.StopProgressIndicator()
	}

	if f.prNumber > 0 {
		if len(opts.Fields) == 1 && opts.Fields[0] == "number" {
			// avoid hitting the API if we already have all the information
			return &api.PullRequest{Number: f.prNumber}, f.repo, nil
		}
		pr, err := findByNumber(httpClient, f.repo, f.prNumber, opts.Fields)
		return pr, f.repo, err
	}

	pr, err := findForBranch(httpClient, f.repo, opts.BaseBranch, f.branchName, opts.States, opts.Fields)

	// TODO: preload view: api.ReviewsForPullRequest, api.CommentsForPullRequest
	// TODO: preload checks: get all checks
	return pr, f.repo, err
}

var pullURLRE = regexp.MustCompile(`^/([^/]+)/([^/]+)/pull/(\d+)`)

func (f *finder) parseURL(prURL string) error {
	if prURL == "" {
		return fmt.Errorf("invalid URL: %q", prURL)
	}

	u, err := url.Parse(prURL)
	if err != nil {
		return err
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	m := pullURLRE.FindStringSubmatch(u.Path)
	if m == nil {
		return fmt.Errorf("not a pull request URL: %s", prURL)
	}

	f.repo = ghrepo.NewWithHost(m[1], m[2], u.Hostname())
	f.prNumber, _ = strconv.Atoi(m[3])
	return nil
}

var prHeadRE = regexp.MustCompile(`^refs/pull/(\d+)/head$`)

func (f *finder) parseCurrentBranch() error {
	prHeadRef, err := f.branchFn()
	if err != nil {
		return err
	}

	branchConfig := git.ReadBranchConfig(prHeadRef)

	// the branch is configured to merge a special PR head ref
	if m := prHeadRE.FindStringSubmatch(branchConfig.MergeRef); m != nil {
		f.prNumber, _ = strconv.Atoi(m[1])
		return nil
	}

	var branchOwner string
	if branchConfig.RemoteURL != nil {
		// the branch merges from a remote specified by URL
		if r, err := ghrepo.FromURL(branchConfig.RemoteURL); err == nil {
			branchOwner = r.RepoOwner()
		}
	} else if branchConfig.RemoteName != "" {
		// the branch merges from a remote specified by name
		rem, _ := f.remotesFn()
		if r, err := rem.FindByName(branchConfig.RemoteName); err == nil {
			branchOwner = r.RepoOwner()
		}
	}

	if branchOwner != "" {
		if strings.HasPrefix(branchConfig.MergeRef, "refs/heads/") {
			prHeadRef = strings.TrimPrefix(branchConfig.MergeRef, "refs/heads/")
		}
		// prepend `OWNER:` if this branch is pushed to a fork
		if !strings.EqualFold(branchOwner, f.repo.RepoOwner()) {
			prHeadRef = fmt.Sprintf("%s:%s", branchOwner, prHeadRef)
		}
	}

	f.branchName = prHeadRef
	return nil
}

func findByNumber(httpClient *http.Client, repo ghrepo.Interface, number int, fields []string) (*api.PullRequest, error) {
	type response struct {
		Repository struct {
			PullRequest api.PullRequest
		}
	}

	query := fmt.Sprintf(`
	query PullRequestByNumber($owner: String!, $repo: String!, $pr_number: Int!) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $pr_number) {%s}
		}
	}`, api.PullRequestGraphQL(fields))

	variables := map[string]interface{}{
		"owner":     repo.RepoOwner(),
		"repo":      repo.RepoName(),
		"pr_number": number,
	}

	var resp response
	client := api.NewClientFromHTTP(httpClient)
	err := client.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return nil, err
	}

	return &resp.Repository.PullRequest, nil
}

func findForBranch(httpClient *http.Client, repo ghrepo.Interface, baseBranch, headBranch string, stateFilters, fields []string) (*api.PullRequest, error) {
	type response struct {
		Repository struct {
			PullRequests struct {
				Nodes []api.PullRequest
			}
		}
	}

	fieldSet := set.NewStringSet()
	fieldSet.AddValues(fields)
	// these fields are required for filtering below
	fieldSet.AddValues([]string{"state", "baseRefName", "headRefName", "isCrossRepository", "headRepositoryOwner"})

	query := fmt.Sprintf(`
	query PullRequestForBranch($owner: String!, $repo: String!, $headRefName: String!, $states: [PullRequestState!]) {
		repository(owner: $owner, name: $repo) {
			pullRequests(headRefName: $headRefName, states: $states, first: 30, orderBy: { field: CREATED_AT, direction: DESC }) {
				nodes {%s}
			}
		}
	}`, api.PullRequestGraphQL(fieldSet.ToSlice()))

	branchWithoutOwner := headBranch
	if idx := strings.Index(headBranch, ":"); idx >= 0 {
		branchWithoutOwner = headBranch[idx+1:]
	}

	variables := map[string]interface{}{
		"owner":       repo.RepoOwner(),
		"repo":        repo.RepoName(),
		"headRefName": branchWithoutOwner,
		"states":      stateFilters,
	}

	var resp response
	client := api.NewClientFromHTTP(httpClient)
	err := client.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return nil, err
	}

	prs := resp.Repository.PullRequests.Nodes
	sort.SliceStable(prs, func(a, b int) bool {
		return prs[a].State == "OPEN" && prs[b].State != "OPEN"
	})

	for _, pr := range prs {
		if pr.HeadLabel() == headBranch && (baseBranch == "" || pr.BaseRefName == baseBranch) {
			return &pr, nil
		}
	}

	return nil, &NotFoundError{fmt.Errorf("no pull requests found for branch %q", headBranch)}
}

type NotFoundError struct {
	error
}

func (err *NotFoundError) Unwrap() error {
	return err.error
}

func NewMockFinder(selector string, pr *api.PullRequest, repo ghrepo.Interface) PRFinder {
	return &mockFinder{
		expectSelector: selector,
		pr:             pr,
		repo:           repo,
	}
}

type mockFinder struct {
	called         bool
	expectSelector string
	pr             *api.PullRequest
	repo           ghrepo.Interface
	err            error
}

func (m *mockFinder) Find(opts FindOptions) (*api.PullRequest, ghrepo.Interface, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	if m.expectSelector != opts.Selector {
		return nil, nil, fmt.Errorf("mockFinder: expected selector %q, got %q", m.expectSelector, opts.Selector)
	}
	if m.called {
		return nil, nil, errors.New("mockFinder used more than once")
	}
	m.called = true

	if m.pr.HeadRepositoryOwner.Login == "" {
		// pose as same-repo PR by default
		m.pr.HeadRepositoryOwner.Login = m.repo.RepoOwner()
	}

	return m.pr, m.repo, nil
}
