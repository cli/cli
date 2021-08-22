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

	issue, err := issueFromNumber(apiClient, baseRepo, issueNumber)
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

func issueFromNumber(apiClient *api.Client, repo ghrepo.Interface, issueNumber int) (*api.Issue, error) {
	return api.IssueByNumber(apiClient, repo, issueNumber)
}
