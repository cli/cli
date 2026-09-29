// Package shared holds what every gh issue artifact command needs before it
// reaches an artifact: its client, its issue argument and the checks that
// refuse what artifacts don't support.
package shared

import (
	"errors"
	"fmt"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/client"
	issueShared "github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

// ClientFunc returns a function that creates an ArtifactClient from the
// factory's HTTP client. Commands store it in their options and call it from
// their run function.
func ClientFunc(f *cmdutil.Factory) func() (client.ArtifactClient, error) {
	return func() (client.ArtifactClient, error) {
		httpClient, err := f.HttpClient()
		if err != nil {
			return nil, err
		}
		return client.NewArtifactClient(httpClient), nil
	}
}

// ParseIssueArg reads an {<issue-number> | <issue-url>} argument the way other
// gh issue commands do, and returns the issue number and the function that
// resolves its repository. A URL names its own repository, so it works outside
// any repository and wins over baseRepo. A number uses baseRepo, which is not
// called here, so -R and GH_REPO still apply. Pull request URLs parse too, so
// CheckIssue can refuse them with a clear message.
func ParseIssueArg(arg string, baseRepo func() (ghrepo.Interface, error)) (int, func() (ghrepo.Interface, error), error) {
	number, repo, err := issueShared.ParseIssueFromArg(arg)
	if err != nil {
		return 0, nil, err
	}
	if r, ok := repo.Value(); ok {
		return number, func() (ghrepo.Interface, error) { return r, nil }, nil
	}
	return number, baseRepo, nil
}

// CheckHost refuses GitHub Enterprise Server, which doesn't support issue
// artifacts. Callers run it before any request, so the user gets this reason
// instead of an API error. ghe.com hosts pass.
func CheckHost(host string) error {
	if ghauth.IsEnterprise(host) {
		return errors.New("issue artifacts are not supported on GitHub Enterprise Server")
	}
	return nil
}

// CheckIssue refuses a pull request, since artifacts are only supported on
// issues. It makes one lookup, and callers run it before any artifact request.
func CheckIssue(c client.ArtifactClient, repo ghrepo.Interface, number int) error {
	isPullRequest, err := c.IsPullRequest(repo, number)
	if err != nil {
		return err
	}
	if isPullRequest {
		return fmt.Errorf("%s#%d is a pull request; artifacts are only supported on issues", ghrepo.FullName(repo), number)
	}
	return nil
}
