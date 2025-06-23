package shared

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	o "github.com/cli/cli/v2/pkg/option"
	"github.com/cli/cli/v2/pkg/set"
	"golang.org/x/sync/errgroup"
)

var issueURLRE = regexp.MustCompile(`^/([^/]+)/([^/]+)/(?:issues|pull)/(\d+)`)

func ParseIssuesFromArgs(args []string) ([]int, o.Option[ghrepo.Interface], error) {
	var repo o.Option[ghrepo.Interface]
	issueNumbers := make([]int, len(args))

	for i, arg := range args {
		// For each argument, parse the issue number and an optional repo
		issueNumber, issueRepo, err := ParseIssueFromArg(arg)
		if err != nil {
			return nil, o.None[ghrepo.Interface](), err
		}

		// if this is our first issue repo found, then we need to set it
		if repo.IsNone() {
			repo = issueRepo
		}

		// if there is an issue repo returned, then we need to check if it is the same as the previous one
		if issueRepo.IsSome() && repo.IsSome() {
			// Unwraps are safe because we've checked for presence above
			if !ghrepo.IsSame(repo.Unwrap(), issueRepo.Unwrap()) {
				return nil, o.None[ghrepo.Interface](), fmt.Errorf(
					"multiple issues must be in same repo: found %q, expected %q",
					ghrepo.FullName(issueRepo.Unwrap()),
					ghrepo.FullName(repo.Unwrap()),
				)
			}
		}

		// add the issue number to the list
		issueNumbers[i] = issueNumber
	}

	return issueNumbers, repo, nil
}

func ParseIssueFromArg(arg string) (int, o.Option[ghrepo.Interface], error) {
	issueLocator := tryParseIssueFromURL(arg)
	if issueLocator, present := issueLocator.Value(); present {
		return issueLocator.issueNumber, o.Some(issueLocator.repo), nil
	}

	issueNumber, err := strconv.Atoi(strings.TrimPrefix(arg, "#"))
	if err != nil {
		return 0, o.None[ghrepo.Interface](), fmt.Errorf("invalid issue format: %q", arg)
	}

	return issueNumber, o.None[ghrepo.Interface](), nil
}

type issueLocator struct {
	issueNumber int
	repo        ghrepo.Interface
}

// tryParseIssueFromURL tries to parse an issue number and repo from a URL.
func tryParseIssueFromURL(maybeURL string) o.Option[issueLocator] {
	u, err := url.Parse(maybeURL)
	if err != nil {
		return o.None[issueLocator]()
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return o.None[issueLocator]()
	}

	m := issueURLRE.FindStringSubmatch(u.Path)
	if m == nil {
		return o.None[issueLocator]()
	}

	repo := ghrepo.NewWithHost(m[1], m[2], u.Hostname())
	issueNumber, _ := strconv.Atoi(m[3])
	return o.Some(issueLocator{
		issueNumber: issueNumber,
		repo:        repo,
	})
}

type PartialLoadError struct {
	error
}

// FindIssuesOrPRs loads 1 or more issues or pull requests with the specified fields. If some of the fields
// could not be fetched by GraphQL, this returns non-nil issues and a *PartialLoadError.
func FindIssuesOrPRs(httpClient *http.Client, repo ghrepo.Interface, issueNumbers []int, fields []string) ([]*api.Issue, error) {
	issuesChan := make(chan *api.Issue, len(issueNumbers))
	g := errgroup.Group{}
	for _, num := range issueNumbers {
		issueNumber := num
		g.Go(func() error {
			issue, err := FindIssueOrPR(httpClient, repo, issueNumber, fields)
			if err != nil {
				return err
			}

			issuesChan <- issue
			return nil
		})
	}

	err := g.Wait()
	close(issuesChan)

	if err != nil {
		return nil, err
	}

	issues := make([]*api.Issue, 0, len(issueNumbers))
	for issue := range issuesChan {
		issues = append(issues, issue)
	}

	return issues, nil
}

func FindIssueOrPR(httpClient *http.Client, repo ghrepo.Interface, number int, fields []string) (*api.Issue, error) {
	fieldSet := set.NewStringSet()
	fieldSet.AddValues(fields)
	if fieldSet.Contains("stateReason") {
		cachedClient := api.NewCachedHTTPClient(httpClient, time.Hour*24)
		detector := fd.NewDetector(cachedClient, repo.RepoHost())
		features, err := detector.IssueFeatures()
		if err != nil {
			return nil, err
		}
		if !features.StateReason {
			fieldSet.Remove("stateReason")
		}
	}

	var getProjectItems bool
	if fieldSet.Contains("projectItems") {
		getProjectItems = true
		fieldSet.Remove("projectItems")
		fieldSet.Add("number")
	}

	fields = fieldSet.ToSlice()

	type response struct {
		Repository struct {
			HasIssuesEnabled bool
			Issue            *api.Issue
		}
	}

	query := fmt.Sprintf(`
	query IssueByNumber($owner: String!, $repo: String!, $number: Int!) {
		repository(owner: $owner, name: $repo) {
			hasIssuesEnabled
			issue: issueOrPullRequest(number: $number) {
				__typename
				...on Issue{%[1]s}
				...on PullRequest{%[2]s}
			}
		}
	}`, api.IssueGraphQL(fields), api.PullRequestGraphQL(fields))

	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"repo":   repo.RepoName(),
		"number": number,
	}

	var resp response
	client := api.NewClientFromHTTP(httpClient)
	if err := client.GraphQL(repo.RepoHost(), query, variables, &resp); err != nil {
		var gerr api.GraphQLError
		if errors.As(err, &gerr) {
			if gerr.Match("NOT_FOUND", "repository.issue") && !resp.Repository.HasIssuesEnabled {
				return nil, fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(repo))
			} else if gerr.Match("FORBIDDEN", "repository.issue.projectCards.") {
				issue := resp.Repository.Issue
				// remove nil entries for project cards due to permission issues
				projects := make([]*api.ProjectInfo, 0, len(issue.ProjectCards.Nodes))
				for _, p := range issue.ProjectCards.Nodes {
					if p != nil {
						projects = append(projects, p)
					}
				}
				issue.ProjectCards.Nodes = projects
				return issue, &PartialLoadError{err}
			}
		}
		return nil, err
	}

	if resp.Repository.Issue == nil {
		return nil, errors.New("issue was not found but GraphQL reported no error")
	}

	if getProjectItems {
		apiClient := api.NewClientFromHTTP(httpClient)
		err := api.ProjectsV2ItemsForIssue(apiClient, repo, resp.Repository.Issue)
		if err != nil && !api.ProjectsV2IgnorableError(err) {
			return nil, err
		}
	}

	return resp.Repository.Issue, nil
}
