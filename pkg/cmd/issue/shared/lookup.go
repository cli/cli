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
	"github.com/cli/cli/v2/pkg/set"
)

// IssueFromArgWithFields loads an issue or pull request with the specified fields. If some of the fields
// could not be fetched by GraphQL, this returns a non-nil issue and a *PartialLoadError.
func IssueFromArgWithFields(httpClient *http.Client, baseRepoFn func() (ghrepo.Interface, error), arg string, fields []string) (*api.Issue, ghrepo.Interface, error) {
	issueNumber, baseRepo := issueMetadataFromURL(arg)

	if issueNumber == 0 {
		var err error
		issueNumber, err = strconv.Atoi(strings.TrimPrefix(arg, "#"))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid issue format: %q", arg)
		}
	}

	if baseRepo == nil {
		var err error
		baseRepo, err = baseRepoFn()
		if err != nil {
			return nil, nil, err
		}
	}

	issue, err := findIssueOrPR(httpClient, baseRepo, issueNumber, fields)
	return issue, baseRepo, err
}

var issueURLRE = regexp.MustCompile(`^/([^/]+)/([^/]+)/issues/(\d+)`)

func issueMetadataFromURL(s string) (int, ghrepo.Interface) {
	u, err := url.Parse(s)
	if err != nil {
		return 0, nil
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return 0, nil
	}

	m := issueURLRE.FindStringSubmatch(u.Path)
	if m == nil {
		return 0, nil
	}

	repo := ghrepo.NewWithHost(m[1], m[2], u.Hostname())
	issueNumber, _ := strconv.Atoi(m[3])
	return issueNumber, repo
}

// Returns the issue number and repo if the issue URL is provided.
// If only the issue number is provided, returns the number and nil repo.
func IssueNumberAndRepoFromArg(arg string) (int, ghrepo.Interface, error) {
	issueNumber, baseRepo := issueMetadataFromURL(arg)

	if issueNumber == 0 {
		var err error
		issueNumber, err = strconv.Atoi(strings.TrimPrefix(arg, "#"))
		if err != nil {
			return 0, nil, fmt.Errorf("invalid issue format: %q", arg)
		}
	}

	return issueNumber, baseRepo, nil
}

type PartialLoadError struct {
	error
}

func findIssueOrPR(httpClient *http.Client, repo ghrepo.Interface, number int, fields []string) (*api.Issue, error) {
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
