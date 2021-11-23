package shared

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

// IssueFromArg loads an issue with all its fields.
// Deprecated: use IssueFromArgWithFields instead.
func IssueFromArg(apiClient *api.Client, baseRepoFn func() (ghrepo.Interface, error), arg string) (*api.Issue, ghrepo.Interface, error) {
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
			return nil, nil, fmt.Errorf("could not determine base repo: %w", err)
		}
	}

	issue, err := api.IssueByNumber(apiClient, baseRepo, issueNumber)
	return issue, baseRepo, err
}

// IssueFromArgWithFields loads an issue or pull request with the specified fields.
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
			return nil, nil, fmt.Errorf("could not determine base repo: %w", err)
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

func findIssueOrPR(httpClient *http.Client, repo ghrepo.Interface, number int, fields []string) (*api.Issue, error) {
	type response struct {
		Repository struct {
			Issue *api.Issue
		}
	}

	query := fmt.Sprintf(`
	query IssueByNumber($owner: String!, $repo: String!, $number: Int!) {
		repository(owner: $owner, name: $repo) {
			issue: issueOrPullRequest(number: $number) {
				...on Issue{%[1]s}
				...on PullRequest{%[1]s}
			}
		}
	}`, api.PullRequestGraphQL(fields))

	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"repo":   repo.RepoName(),
		"number": number,
	}

	var resp response
	client := api.NewClientFromHTTP(httpClient)
	if err := client.GraphQL(repo.RepoHost(), query, variables, &resp); err != nil {
		return nil, err
	}

	return resp.Repository.Issue, nil
}
