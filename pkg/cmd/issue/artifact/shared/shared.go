// Package shared holds what more than one gh issue artifact command needs: its
// client, its arguments, reading new content from files, finding a version in
// an edit history, the result lines of commands that act on several artifacts
// and the checks that refuse what artifacts or this version of gh don't
// support.
package shared

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

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

// ParseArtifactNumber reads an <artifact-number> argument, which must be a
// whole number of at least 1, so a mistyped number fails before any request.
func ParseArtifactNumber(arg string) (int, error) {
	number, err := strconv.Atoi(arg)
	if err != nil || number < 1 {
		return 0, fmt.Errorf("invalid artifact number: %q", arg)
	}
	return number, nil
}

// FindVersion returns the version numbered number from an artifact's edit
// history, or nil for the current version. Versions come newest first, so the
// newest version's number means the current version, like 0 does. A number
// the API didn't return is an error.
func FindVersion(a *client.ArtifactWithVersions, number int) (*client.Version, error) {
	if number == 0 {
		return nil, nil
	}
	i := slices.IndexFunc(a.Versions, func(v client.Version) bool { return v.Version == number })
	switch {
	case i < 0:
		return nil, fmt.Errorf("version %d not found for artifact %d", number, a.Number)
	case i == 0:
		return nil, nil
	}
	return &a.Versions[i], nil
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
		return pullRequestError(repo, number)
	}
	return nil
}

// CheckUploadTarget is CheckIssue for a command that uploads files with
// --attach. It refuses a pull request the same way, and its one lookup also
// returns the repository ID and permission that attachments.NewUploader
// checks.
func CheckUploadTarget(c client.ArtifactClient, repo ghrepo.Interface, number int) (*client.UploadTarget, error) {
	target, err := c.UploadTarget(repo, number)
	if err != nil {
		return nil, err
	}
	if target.IsPullRequest {
		return nil, pullRequestError(repo, number)
	}
	return target, nil
}

func pullRequestError(repo ghrepo.Interface, number int) error {
	return fmt.Errorf("%s#%d is a pull request; artifacts are only supported on issues", ghrepo.FullName(repo), number)
}

// CheckType refuses an artifact whose type this version of gh doesn't know. A
// new type can need handling gh doesn't have, so commands whose behavior
// depends on the type refuse it instead of guessing. Commands that work the
// same for every type, such as list, don't call it.
func CheckType(a client.Artifact) error {
	switch a.Type {
	case client.TypeGeneric, client.TypePlan, client.TypeLink:
		return nil
	}
	return fmt.Errorf("artifact %d has type %q, which this version of gh doesn't support; upgrade gh", a.Number, a.Type)
}

// HTTPURL returns value with surrounding whitespace trimmed, and whether it is
// an http(s) URL: an absolute http or https URL with a host, in printable
// ASCII with no spaces. Artifact commands use this one check wherever they read
// or write a link. It is stricter than the server's because links are also
// stored in Internet Shortcut files, whose URL= line can't hold anything else.
func HTTPURL(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] <= ' ' || trimmed[i] > '~' {
			return "", false
		}
	}
	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", false
	}
	return trimmed, true
}
