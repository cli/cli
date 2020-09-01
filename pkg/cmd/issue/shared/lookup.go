package shared

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
)

func IssueFromArg(apiClient *api.Client, baseRepoFn func() (ghrepo.Interface, error), arg string) (*api.Issue, ghrepo.Interface, error) {
	issue, baseRepo, err := issueFromURL(apiClient, arg)
	if err != nil {
		return nil, nil, err
	}
	if issue != nil {
		return issue, baseRepo, nil
	}

	baseRepo, err = baseRepoFn()
	if err != nil {
		return nil, nil, fmt.Errorf("could not determine base repo: %w", err)
	}

	issueNumber, err := strconv.Atoi(strings.TrimPrefix(arg, "#"))
	if err != nil {
		return nil, nil, fmt.Errorf("invalid issue format: %q", arg)
	}

	issue, err = issueFromNumber(apiClient, baseRepo, issueNumber)
	return issue, baseRepo, err
}

var issueURLRE = regexp.MustCompile(`^/([^/]+)/([^/]+)/issues/(\d+)`)

func issueFromURL(apiClient *api.Client, s string) (*api.Issue, ghrepo.Interface, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, nil, nil
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, nil, nil
	}

	m := issueURLRE.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, nil, nil
	}

	repo := ghrepo.NewWithHost(m[1], m[2], u.Hostname())
	issueNumber, _ := strconv.Atoi(m[3])
	issue, err := issueFromNumber(apiClient, repo, issueNumber)
	return issue, repo, err
}

func issueFromNumber(apiClient *api.Client, repo ghrepo.Interface, issueNumber int) (*api.Issue, error) {
	return api.IssueByNumber(apiClient, repo, issueNumber)
}
