package shared

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	o "github.com/cli/cli/v2/pkg/option"
	"github.com/cli/cli/v2/pkg/set"
	"github.com/shurcooL/githubv4"
	"golang.org/x/sync/errgroup"
)

type PRFinder interface {
	Find(opts FindOptions) (*api.PullRequest, ghrepo.Interface, error)
}

type progressIndicator interface {
	StartProgressIndicator()
	StopProgressIndicator()
}

type GitConfigClient interface {
	ReadBranchConfig(ctx context.Context, branchName string) (git.BranchConfig, error)
	PushDefault(ctx context.Context) (git.PushDefault, error)
	RemotePushDefault(ctx context.Context) (string, error)
	PushRevision(ctx context.Context, branchName string) (git.RemoteTrackingRef, error)
}

type finder struct {
	baseRepoFn      func() (ghrepo.Interface, error)
	branchFn        func() (string, error)
	httpClient      func() (*http.Client, error)
	remotesFn       func() (ghContext.Remotes, error)
	gitConfigClient GitConfigClient
	progress        progressIndicator

	baseRefRepo ghrepo.Interface
	prNumber    int
	branchName  string
}

func NewFinder(factory *cmdutil.Factory) PRFinder {
	if runCommandFinder != nil {
		f := runCommandFinder
		runCommandFinder = &mockFinder{err: errors.New("you must use a RunCommandFinder to stub PR lookups")}
		return f
	}

	return &finder{
		baseRepoFn:      factory.BaseRepo,
		branchFn:        factory.Branch,
		httpClient:      factory.HttpClient,
		gitConfigClient: factory.GitClient,
		remotesFn:       factory.Remotes,
		progress:        factory.IOStreams,
	}
}

var runCommandFinder PRFinder

// RunCommandFinder is the NewMockFinder substitute to be used ONLY in runCommand-style tests.
func RunCommandFinder(selector string, pr *api.PullRequest, repo ghrepo.Interface) *mockFinder {
	finder := NewMockFinder(selector, pr, repo)
	runCommandFinder = finder
	return finder
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
	// If we have a URL, we don't need git stuff
	if len(opts.Fields) == 0 {
		return nil, nil, errors.New("Find error: no fields specified")
	}

	if repo, prNumber, err := f.parseURL(opts.Selector); err == nil {
		f.prNumber = prNumber
		f.baseRefRepo = repo
	}

	if f.baseRefRepo == nil {
		repo, err := f.baseRepoFn()
		if err != nil {
			return nil, nil, err
		}
		f.baseRefRepo = repo
	}

	var prRefs PRFindRefs
	if opts.Selector == "" {
		// You must be in a git repo for this case to work
		currentBranchName, err := f.branchFn()
		if err != nil {
			return nil, nil, err
		}
		f.branchName = currentBranchName

		// Get the branch config for the current branchName
		branchConfig, err := f.gitConfigClient.ReadBranchConfig(context.Background(), f.branchName)
		if err != nil {
			return nil, nil, err
		}

		// Determine if the branch is configured to merge to a special PR ref
		prHeadRE := regexp.MustCompile(`^refs/pull/(\d+)/head$`)
		if m := prHeadRE.FindStringSubmatch(branchConfig.MergeRef); m != nil {
			prNumber, _ := strconv.Atoi(m[1])
			f.prNumber = prNumber
		}

		// Determine the PullRequestRefs from config
		if f.prNumber == 0 {
			prRefsResolver := NewPullRequestFindRefsResolver(
				// We requested the branch config already, so let's cache that
				CachedBranchConfigGitConfigClient{
					CachedBranchConfig: branchConfig,
					GitConfigClient:    f.gitConfigClient,
				},
				f.remotesFn,
			)
			prRefs, err = prRefsResolver.ResolvePullRequestRefs(f.baseRefRepo, opts.BaseBranch, f.branchName)
			if err != nil {
				return nil, nil, err
			}
		}
	} else if f.prNumber == 0 {
		// You gave me a selector but I couldn't find a PR number (it wasn't a URL)

		// Try to get a PR number from the selector
		prNumber, err := strconv.Atoi(strings.TrimPrefix(opts.Selector, "#"))
		// If opts.Selector is a valid number then assume it is the
		// PR number unless opts.BaseBranch is specified. This is a
		// special case for PR create command which will always want
		// to assume that a numerical selector is a branch name rather
		// than PR number.
		if opts.BaseBranch == "" && err == nil {
			f.prNumber = prNumber
		} else {
			f.branchName = opts.Selector

			qualifiedHeadRef, err := ParseQualifiedHeadRef(f.branchName)
			if err != nil {
				return nil, nil, err
			}

			prRefs = PRFindRefs{
				qualifiedHeadRef: qualifiedHeadRef,
				baseRepo:         f.baseRefRepo,
				baseBranchName:   o.SomeIfNonZero(opts.BaseBranch),
			}
		}
	}

	// Set up HTTP client
	httpClient, err := f.httpClient()
	if err != nil {
		return nil, nil, err
	}

	// TODO(josebalius): Should we be guarding here?
	if f.progress != nil {
		f.progress.StartProgressIndicator()
		defer f.progress.StopProgressIndicator()
	}

	fields := set.NewStringSet()
	fields.AddValues(opts.Fields)
	numberFieldOnly := fields.Len() == 1 && fields.Contains("number")
	fields.AddValues([]string{"id", "number"}) // for additional preload queries below

	if fields.Contains("isInMergeQueue") || fields.Contains("isMergeQueueEnabled") {
		cachedClient := api.NewCachedHTTPClient(httpClient, time.Hour*24)
		detector := fd.NewDetector(cachedClient, f.baseRefRepo.RepoHost())
		prFeatures, err := detector.PullRequestFeatures()
		if err != nil {
			return nil, nil, err
		}
		if !prFeatures.MergeQueue {
			fields.Remove("isInMergeQueue")
			fields.Remove("isMergeQueueEnabled")
		}
	}

	var getProjectItems bool
	if fields.Contains("projectItems") {
		getProjectItems = true
		fields.Remove("projectItems")
	}

	var pr *api.PullRequest
	if f.prNumber > 0 {
		if numberFieldOnly {
			// avoid hitting the API if we already have all the information
			return &api.PullRequest{Number: f.prNumber}, f.baseRefRepo, nil
		}
		pr, err = findByNumber(httpClient, f.baseRefRepo, f.prNumber, fields.ToSlice())
		if err != nil {
			return pr, f.baseRefRepo, err
		}
	} else {
		pr, err = findForRefs(httpClient, prRefs, opts.States, fields.ToSlice())
		if err != nil {
			return pr, f.baseRefRepo, err
		}
	}

	g, _ := errgroup.WithContext(context.Background())
	if fields.Contains("reviews") {
		g.Go(func() error {
			return preloadPrReviews(httpClient, f.baseRefRepo, pr)
		})
	}
	if fields.Contains("comments") {
		g.Go(func() error {
			return preloadPrComments(httpClient, f.baseRefRepo, pr)
		})
	}
	if fields.Contains("closingIssuesReferences") {
		g.Go(func() error {
			return preloadPrClosingIssuesReferences(httpClient, f.baseRefRepo, pr)
		})
	}
	if fields.Contains("statusCheckRollup") {
		g.Go(func() error {
			return preloadPrChecks(httpClient, f.baseRefRepo, pr)
		})
	}
	if getProjectItems {
		g.Go(func() error {
			apiClient := api.NewClientFromHTTP(httpClient)
			err := api.ProjectsV2ItemsForPullRequest(apiClient, f.baseRefRepo, pr)
			if err != nil && !api.ProjectsV2IgnorableError(err) {
				return err
			}
			return nil
		})
	}

	return pr, f.baseRefRepo, g.Wait()
}

var pullURLRE = regexp.MustCompile(`^/([^/]+)/([^/]+)/pull/(\d+)`)

func (f *finder) parseURL(prURL string) (ghrepo.Interface, int, error) {
	if prURL == "" {
		return nil, 0, fmt.Errorf("invalid URL: %q", prURL)
	}

	u, err := url.Parse(prURL)
	if err != nil {
		return nil, 0, err
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, 0, fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	m := pullURLRE.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, 0, fmt.Errorf("not a pull request URL: %s", prURL)
	}

	repo := ghrepo.NewWithHost(m[1], m[2], u.Hostname())
	prNumber, _ := strconv.Atoi(m[3])
	return repo, prNumber, nil
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

func findForRefs(httpClient *http.Client, prRefs PRFindRefs, stateFilters, fields []string) (*api.PullRequest, error) {
	type response struct {
		Repository struct {
			PullRequests struct {
				Nodes []api.PullRequest
			}
			DefaultBranchRef struct {
				Name string
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
			defaultBranchRef { name }
		}
	}`, api.PullRequestGraphQL(fieldSet.ToSlice()))

	variables := map[string]interface{}{
		"owner":       prRefs.BaseRepo().RepoOwner(),
		"repo":        prRefs.BaseRepo().RepoName(),
		"headRefName": prRefs.UnqualifiedHeadRef(),
		"states":      stateFilters,
	}

	var resp response
	client := api.NewClientFromHTTP(httpClient)
	err := client.GraphQL(prRefs.BaseRepo().RepoHost(), query, variables, &resp)
	if err != nil {
		return nil, err
	}

	prs := resp.Repository.PullRequests.Nodes
	sort.SliceStable(prs, func(a, b int) bool {
		return prs[a].State == "OPEN" && prs[b].State != "OPEN"
	})

	for _, pr := range prs {
		// When the head is the default branch, it doesn't really make sense to show merged or closed PRs.
		// https://github.com/cli/cli/issues/4263
		isNotClosedOrMergedWhenHeadIsDefault := pr.State == "OPEN" || resp.Repository.DefaultBranchRef.Name != prRefs.QualifiedHeadRef()
		if prRefs.Matches(pr.BaseRefName, pr.HeadLabel()) && isNotClosedOrMergedWhenHeadIsDefault {
			return &pr, nil
		}
	}

	return nil, &NotFoundError{fmt.Errorf("no pull requests found for branch %q", prRefs.QualifiedHeadRef())}
}

func preloadPrReviews(httpClient *http.Client, repo ghrepo.Interface, pr *api.PullRequest) error {
	if !pr.Reviews.PageInfo.HasNextPage {
		return nil
	}

	type response struct {
		Node struct {
			PullRequest struct {
				Reviews api.PullRequestReviews `graphql:"reviews(first: 100, after: $endCursor)"`
			} `graphql:"...on PullRequest"`
		} `graphql:"node(id: $id)"`
	}

	variables := map[string]interface{}{
		"id":        githubv4.ID(pr.ID),
		"endCursor": githubv4.String(pr.Reviews.PageInfo.EndCursor),
	}

	gql := api.NewClientFromHTTP(httpClient)

	for {
		var query response
		err := gql.Query(repo.RepoHost(), "ReviewsForPullRequest", &query, variables)
		if err != nil {
			return err
		}

		pr.Reviews.Nodes = append(pr.Reviews.Nodes, query.Node.PullRequest.Reviews.Nodes...)
		pr.Reviews.TotalCount = len(pr.Reviews.Nodes)

		if !query.Node.PullRequest.Reviews.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Node.PullRequest.Reviews.PageInfo.EndCursor)
	}

	pr.Reviews.PageInfo.HasNextPage = false
	return nil
}

func preloadPrComments(client *http.Client, repo ghrepo.Interface, pr *api.PullRequest) error {
	if !pr.Comments.PageInfo.HasNextPage {
		return nil
	}

	type response struct {
		Node struct {
			PullRequest struct {
				Comments api.Comments `graphql:"comments(first: 100, after: $endCursor)"`
			} `graphql:"...on PullRequest"`
		} `graphql:"node(id: $id)"`
	}

	variables := map[string]interface{}{
		"id":        githubv4.ID(pr.ID),
		"endCursor": githubv4.String(pr.Comments.PageInfo.EndCursor),
	}

	gql := api.NewClientFromHTTP(client)

	for {
		var query response
		err := gql.Query(repo.RepoHost(), "CommentsForPullRequest", &query, variables)
		if err != nil {
			return err
		}

		pr.Comments.Nodes = append(pr.Comments.Nodes, query.Node.PullRequest.Comments.Nodes...)
		pr.Comments.TotalCount = len(pr.Comments.Nodes)

		if !query.Node.PullRequest.Comments.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Node.PullRequest.Comments.PageInfo.EndCursor)
	}

	pr.Comments.PageInfo.HasNextPage = false
	return nil
}

func preloadPrClosingIssuesReferences(client *http.Client, repo ghrepo.Interface, pr *api.PullRequest) error {
	if !pr.ClosingIssuesReferences.PageInfo.HasNextPage {
		return nil
	}

	type response struct {
		Node struct {
			PullRequest struct {
				ClosingIssuesReferences api.ClosingIssuesReferences `graphql:"closingIssuesReferences(first: 100, after: $endCursor)"`
			} `graphql:"...on PullRequest"`
		} `graphql:"node(id: $id)"`
	}

	variables := map[string]interface{}{
		"id":        githubv4.ID(pr.ID),
		"endCursor": githubv4.String(pr.ClosingIssuesReferences.PageInfo.EndCursor),
	}

	gql := api.NewClientFromHTTP(client)

	for {
		var query response
		err := gql.Query(repo.RepoHost(), "closingIssuesReferences", &query, variables)
		if err != nil {
			return err
		}

		pr.ClosingIssuesReferences.Nodes = append(pr.ClosingIssuesReferences.Nodes, query.Node.PullRequest.ClosingIssuesReferences.Nodes...)

		if !query.Node.PullRequest.ClosingIssuesReferences.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Node.PullRequest.ClosingIssuesReferences.PageInfo.EndCursor)
	}

	pr.ClosingIssuesReferences.PageInfo.HasNextPage = false
	return nil
}

func preloadPrChecks(client *http.Client, repo ghrepo.Interface, pr *api.PullRequest) error {
	if len(pr.StatusCheckRollup.Nodes) == 0 {
		return nil
	}
	statusCheckRollup := &pr.StatusCheckRollup.Nodes[0].Commit.StatusCheckRollup.Contexts
	if !statusCheckRollup.PageInfo.HasNextPage {
		return nil
	}

	endCursor := statusCheckRollup.PageInfo.EndCursor

	type response struct {
		Node *api.PullRequest
	}

	query := fmt.Sprintf(`
	query PullRequestStatusChecks($id: ID!, $endCursor: String!) {
		node(id: $id) {
			...on PullRequest {
				%s
			}
		}
	}`, api.StatusCheckRollupGraphQLWithoutCountByState("$endCursor"))

	variables := map[string]interface{}{
		"id": pr.ID,
	}

	apiClient := api.NewClientFromHTTP(client)
	for {
		variables["endCursor"] = endCursor
		var resp response
		err := apiClient.GraphQL(repo.RepoHost(), query, variables, &resp)
		if err != nil {
			return err
		}

		result := resp.Node.StatusCheckRollup.Nodes[0].Commit.StatusCheckRollup.Contexts
		statusCheckRollup.Nodes = append(
			statusCheckRollup.Nodes,
			result.Nodes...,
		)

		if !result.PageInfo.HasNextPage {
			break
		}
		endCursor = result.PageInfo.EndCursor
	}

	statusCheckRollup.PageInfo.HasNextPage = false
	return nil
}

type NotFoundError struct {
	error
}

func (err *NotFoundError) Unwrap() error {
	return err.error
}

func NewMockFinder(selector string, pr *api.PullRequest, repo ghrepo.Interface) *mockFinder {
	var err error
	if pr == nil {
		err = &NotFoundError{errors.New("no pull requests found")}
	}
	return &mockFinder{
		expectSelector: selector,
		pr:             pr,
		repo:           repo,
		err:            err,
	}
}

type mockFinder struct {
	called         bool
	expectSelector string
	expectFields   []string
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
	if len(m.expectFields) > 0 && !isEqualSet(m.expectFields, opts.Fields) {
		return nil, nil, fmt.Errorf("mockFinder: expected fields %v, got %v", m.expectFields, opts.Fields)
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

func (m *mockFinder) ExpectFields(fields []string) {
	m.expectFields = fields
}

func isEqualSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	aCopy := make([]string, len(a))
	copy(aCopy, a)
	bCopy := make([]string, len(b))
	copy(bCopy, b)
	sort.Strings(aCopy)
	sort.Strings(bCopy)

	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}
	return true
}
