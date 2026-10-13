package shared

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/internal/attachments"
	"github.com/cli/cli/v2/internal/gh"
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

// MissingArtifactMessage is the usage error for a command that needs an
// artifact when none is named: no arguments, or a lone argument that isn't an
// artifact URL, such as an issue alone. It's worded like gh issue develop's
// "issue number or url is required".
const MissingArtifactMessage = "artifact number or URL is required"

// ParseArtifactArgs reads the arguments of a command that names artifacts in
// either of two forms, {<issue> <artifact-number>... | <artifact-url>...}, and
// returns the issue number, the function that resolves its repository, and
// the artifact numbers in argument order. args must not be empty, which the
// command's Args check ensures.
//
// The first argument decides the form. When it's an artifact URL, every
// argument must be one, all on the same issue, since the command's results
// name one issue. An artifact URL names its own repository, like an issue URL
// in ParseIssueArg, and any URL that goes on past its issue number is read as
// one. Otherwise, the first argument is the issue, read by ParseIssueArg, and
// the rest are artifact numbers. Either way, a repeated number is an error,
// since a second request for it could only fail or repeat the first.
//
// With issueAlone, a lone issue argument names every artifact on it, and the
// numbers are empty. Without it, a lone argument that isn't an artifact URL
// names no artifact, which fails with MissingArtifactMessage.
func ParseArtifactArgs(args []string, baseRepo func() (ghrepo.Interface, error), issueAlone bool) (int, func() (ghrepo.Interface, error), []int, error) {
	if isArtifactURL(args[0]) {
		return parseArtifactURLs(args)
	}
	if len(args) == 1 && !issueAlone {
		return 0, nil, nil, errors.New(MissingArtifactMessage)
	}

	issueNumber, repo, err := ParseIssueArg(args[0], baseRepo)
	if err != nil {
		return 0, nil, nil, err
	}
	numbers := make([]int, 0, len(args)-1)
	for _, arg := range args[1:] {
		number, err := ParseArtifactNumber(arg)
		if err != nil {
			return 0, nil, nil, err
		}
		if numbers, err = appendNew(numbers, number); err != nil {
			return 0, nil, nil, err
		}
	}
	return issueNumber, repo, numbers, nil
}

// ParseArtifactArg is ParseArtifactArgs for a command that takes one
// artifact, {<issue> <artifact-number> | <artifact-url>}. A second artifact
// URL is always one too many, so it's refused before the URLs' issues and
// numbers are compared.
func ParseArtifactArg(args []string, baseRepo func() (ghrepo.Interface, error)) (int, func() (ghrepo.Interface, error), int, error) {
	if len(args) > 1 && isArtifactURL(args[0]) && isArtifactURL(args[1]) {
		return 0, nil, 0, errors.New("too many arguments")
	}
	issueNumber, repo, numbers, err := ParseArtifactArgs(args, baseRepo, false)
	if err != nil {
		return 0, nil, 0, err
	}
	if len(numbers) != 1 {
		return 0, nil, 0, errors.New("too many arguments")
	}
	return issueNumber, repo, numbers[0], nil
}

// artifactURLPath matches the path of an artifact's page on GitHub, the URL
// the API returns for it: /<owner>/<repo>/issues/<issue>/artifacts/<number>.
var artifactURLPath = regexp.MustCompile(`^/([^/]+)/([^/]+)/issues/(\d+)/artifacts/(\d+)/?$`)

// pastIssuePath matches a path that goes on past its issue or pull request
// number. ParseIssueArg reads such a URL as its issue, but here it can only be
// meant as an artifact URL, so a mistyped one fails instead of naming its
// whole issue.
var pastIssuePath = regexp.MustCompile(`^/[^/]+/[^/]+/(?:issues|pull)/[^/]+/.`)

// isArtifactURL reports whether arg is meant as an artifact URL: an http(s)
// URL whose path goes on past its issue number. parseArtifactURL decides
// whether it's a valid one.
func isArtifactURL(arg string) bool {
	u, err := url.Parse(arg)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && pastIssuePath.MatchString(u.Path)
}

// parseArtifactURLs reads every argument as an artifact URL on the same
// issue.
func parseArtifactURLs(args []string) (int, func() (ghrepo.Interface, error), []int, error) {
	var repo ghrepo.Interface
	var issueNumber int
	numbers := make([]int, 0, len(args))
	for _, arg := range args {
		r, issue, number, err := parseArtifactURL(arg)
		if err != nil {
			return 0, nil, nil, err
		}
		if repo == nil {
			repo, issueNumber = r, issue
		} else if issue != issueNumber || !ghrepo.IsSame(r, repo) {
			return 0, nil, nil, errors.New("artifact URLs must all be on the same issue")
		}
		if numbers, err = appendNew(numbers, number); err != nil {
			return 0, nil, nil, err
		}
	}
	return issueNumber, func() (ghrepo.Interface, error) { return repo, nil }, numbers, nil
}

// parseArtifactURL reads an artifact URL into its repository, issue number
// and artifact number. Like an issue URL, it may be http, and its query and
// fragment are ignored: only those three are read from it, and requests go to
// the API.
func parseArtifactURL(arg string) (ghrepo.Interface, int, int, error) {
	invalid := fmt.Errorf("invalid artifact URL: %q", arg)
	u, err := url.Parse(arg)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, 0, 0, invalid
	}
	m := artifactURLPath.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, 0, 0, invalid
	}
	issueNumber, err := strconv.Atoi(m[3])
	if err != nil || issueNumber < 1 {
		return nil, 0, 0, invalid
	}
	number, err := strconv.Atoi(m[4])
	if err != nil || number < 1 {
		return nil, 0, 0, invalid
	}
	return ghrepo.NewWithHost(m[1], m[2], u.Hostname()), issueNumber, number, nil
}

// appendNew appends number to numbers, refusing a number already there.
func appendNew(numbers []int, number int) ([]int, error) {
	if slices.Contains(numbers, number) {
		return nil, fmt.Errorf("duplicate artifact number: %d", number)
	}
	return append(numbers, number), nil
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

// NewUploader returns the uploader for a command's --attach files, which
// upload against the issue's repository with the token for its host. It
// refuses a pull request like CheckIssue, from the one lookup that also
// returns the repository ID and permission an upload needs, so a caller makes
// no other lookup.
func NewUploader(c client.ArtifactClient, repo ghrepo.Interface, issueNumber int, httpClient func() (*http.Client, error), config func() (gh.Config, error)) (*attachments.Uploader, error) {
	target, err := CheckUploadTarget(c, repo, issueNumber)
	if err != nil {
		return nil, err
	}
	hc, err := httpClient()
	if err != nil {
		return nil, err
	}
	cfg, err := config()
	if err != nil {
		return nil, err
	}
	host := repo.RepoHost()
	return attachments.NewUploader(hc, cfg.Authentication().ActiveTokenType(host), host, target.RepositoryID, target.ViewerPermission)
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

// SupportsAttachments reports whether an artifact of type artifactType can
// take --attach files. Attachments go in Markdown, which only generic and plan
// artifacts hold, so a type gh learns later supports them only once it's
// added here.
func SupportsAttachments(artifactType string) bool {
	switch artifactType {
	case client.TypeGeneric, client.TypePlan:
		return true
	}
	return false
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
