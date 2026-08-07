package shared

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
)

var ReleaseFields = []string{
	"apiUrl",
	"author",
	"assets",
	"body",
	"createdAt",
	"databaseId",
	"id",
	"isDraft",
	"isPrerelease",
	"isImmutable",
	"name",
	"publishedAt",
	"tagName",
	"tarballUrl",
	"targetCommitish",
	"uploadUrl",
	"url",
	"zipballUrl",
}

type Release struct {
	DatabaseID   int64      `json:"id"`
	ID           string     `json:"node_id"`
	TagName      string     `json:"tag_name"`
	Name         string     `json:"name"`
	Body         string     `json:"body"`
	IsDraft      bool       `json:"draft"`
	IsPrerelease bool       `json:"prerelease"`
	IsImmutable  bool       `json:"immutable"`
	CreatedAt    time.Time  `json:"created_at"`
	PublishedAt  *time.Time `json:"published_at"`

	TargetCommitish string `json:"target_commitish"`

	APIURL     string `json:"url"`
	UploadURL  string `json:"upload_url"`
	TarballURL string `json:"tarball_url"`
	ZipballURL string `json:"zipball_url"`
	URL        string `json:"html_url"`
	Assets     []ReleaseAsset

	Author struct {
		ID    string `json:"node_id"`
		Login string `json:"login"`
	}
}

type ReleaseAsset struct {
	ID     string `json:"node_id"`
	Name   string
	Label  string
	Size   int64
	Digest *string
	State  string
	APIURL string `json:"url"`

	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	DownloadCount      int       `json:"download_count"`
	ContentType        string    `json:"content_type"`
	BrowserDownloadURL string    `json:"browser_download_url"`
}

func (rel *Release) ExportData(fields []string) map[string]any {
	v := reflect.ValueOf(rel).Elem()
	fieldByName := func(v reflect.Value, field string) reflect.Value {
		return v.FieldByNameFunc(func(s string) bool {
			return strings.EqualFold(field, s)
		})
	}
	data := map[string]any{}

	for _, f := range fields {
		switch f {
		case "author":
			data[f] = map[string]any{
				"id":    rel.Author.ID,
				"login": rel.Author.Login,
			}
		case "assets":
			assets := make([]any, 0, len(rel.Assets))
			for _, a := range rel.Assets {
				assets = append(assets, map[string]any{
					"url":           a.BrowserDownloadURL,
					"apiUrl":        a.APIURL,
					"id":            a.ID,
					"name":          a.Name,
					"label":         a.Label,
					"size":          a.Size,
					"digest":        a.Digest,
					"state":         a.State,
					"createdAt":     a.CreatedAt,
					"updatedAt":     a.UpdatedAt,
					"downloadCount": a.DownloadCount,
					"contentType":   a.ContentType,
				})
			}
			data[f] = assets
		default:
			sf := fieldByName(v, f)
			data[f] = sf.Interface()
		}
	}

	return data
}

var ErrReleaseNotFound = errors.New("release not found")

type fetchResult struct {
	release *Release
	error   error
}

func FetchRefSHA(ctx context.Context, httpClient *http.Client, repo ghrepo.Interface, tagName string) (string, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "git", "ref", fmt.Sprintf("tags/%s", tagName))
	if err != nil {
		return "", err
	}

	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, err := api.NewClientFromHTTP(httpClient).RequestWithContext(ctx, repo.RepoHost(), http.MethodGet, path.String(), nil)
	if err != nil {
		if isNotFound(err) {
			return "", ErrReleaseNotFound
		}
		return "", err
	}
	defer resp.Body.Close()

	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ref); err != nil {
		return "", fmt.Errorf("failed to parse ref response: %w", err)
	}

	return ref.Object.SHA, nil
}

// DigestAlgForRef returns the digest algorithm name corresponding to the given
// git ref SHA. SHA-1 git object IDs are 40 hex characters and SHA-256 git
// object IDs are 64 hex characters. Unknown lengths default to "sha1" to
// preserve backwards-compatible behavior.
func DigestAlgForRef(digest string) string {
	switch len(digest) {
	case 64:
		return "sha256"
	default:
		return "sha1"
	}
}

// FetchRelease finds a published repository release by its tagName, or a draft release by its pending tag name.
func FetchRelease(ctx context.Context, httpClient *http.Client, repo ghrepo.Interface, tagName string) (*Release, error) {
	publishedPath, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases", "tags", tagName)
	if err != nil {
		return nil, err
	}

	cc, cancel := context.WithCancel(ctx)
	results := make(chan fetchResult, 2)

	// published release lookup
	go func() {
		release, err := fetchReleasePath(cc, httpClient, repo.RepoHost(), publishedPath)
		results <- fetchResult{release: release, error: err}
	}()

	// draft release lookup
	go func() {
		release, err := fetchDraftRelease(cc, httpClient, repo, tagName)
		results <- fetchResult{release: release, error: err}
	}()

	// Prefer a release found by either lookup. A single failed lookup, such as
	// the draft lookup when unauthenticated, must not mask a release found by
	// the other; only report an error when both lookups fail.
	first := <-results
	if first.error == nil {
		cancel()
		<-results // drain the channel
		return first.release, nil
	}

	second := <-results
	cancel() // satisfy the linter even though no goroutines are running anymore
	if second.error == nil {
		return second.release, nil
	}

	// Both lookups failed; prefer reporting the release as not found.
	if errors.Is(second.error, ErrReleaseNotFound) {
		return nil, second.error
	}
	return nil, first.error
}

// FetchLatestRelease finds the latest published release for a repository.
func FetchLatestRelease(ctx context.Context, httpClient *http.Client, repo ghrepo.Interface) (*Release, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases", "latest")
	if err != nil {
		return nil, err
	}
	return fetchReleasePath(ctx, httpClient, repo.RepoHost(), path)
}

// fetchDraftRelease returns the first draft release that has tagName as its pending tag.
func fetchDraftRelease(ctx context.Context, httpClient *http.Client, repo ghrepo.Interface, tagName string) (*Release, error) {
	// First use GraphQL to find a draft release by pending tag name, since REST doesn't have this ability.
	var query struct {
		Repository struct {
			Release *struct {
				DatabaseID int64
				IsDraft    bool
			} `graphql:"release(tagName: $tagName)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]any{
		"owner":   githubv4.String(repo.RepoOwner()),
		"name":    githubv4.String(repo.RepoName()),
		"tagName": githubv4.String(tagName),
	}

	gql := api.NewClientFromHTTP(httpClient)
	if err := gql.QueryWithContext(ctx, repo.RepoHost(), "RepositoryReleaseByTag", &query, variables); err != nil {
		return nil, err
	}

	if query.Repository.Release == nil || !query.Repository.Release.IsDraft {
		return nil, ErrReleaseNotFound
	}

	// Then, use REST to get information about the draft release. In theory, we could have fetched
	// all the necessary information via GraphQL, but REST is safer for backwards compatibility.
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases", strconv.FormatInt(query.Repository.Release.DatabaseID, 10))
	if err != nil {
		return nil, err
	}
	return fetchReleasePath(ctx, httpClient, repo.RepoHost(), path)
}

func fetchReleasePath(ctx context.Context, httpClient *http.Client, host string, path safeurl.SafeURL) (*Release, error) {
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, err := api.NewClientFromHTTP(httpClient).RequestWithContext(ctx, host, http.MethodGet, path.String(), nil)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrReleaseNotFound
		}
		return nil, err
	}
	defer resp.Body.Close()

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

// isNotFound reports whether err is an API error carrying a 404 status. The release lookups
// treat a missing release as a sentinel rather than an error, and the api client reports the
// status through an HTTPError rather than through a response the call site can inspect.
func isNotFound(err error) bool {
	var httpErr api.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}

func StubFetchRelease(t *testing.T, reg *httpmock.Registry, owner, repoName, tagName, responseBody string) {
	path := "repos/OWNER/REPO/releases/tags/v1.2.3"
	if tagName == "" {
		path = "repos/OWNER/REPO/releases/latest"
	}

	reg.Register(httpmock.REST("GET", path), httpmock.StringResponse(responseBody))

	if tagName != "" {
		reg.Register(
			httpmock.GraphQL(`query RepositoryReleaseByTag\b`),
			httpmock.GraphQLQuery(`{ "data": { "repository": { "release": null }}}`,
				func(q string, vars map[string]any) {
					assert.Equal(t, owner, vars["owner"])
					assert.Equal(t, repoName, vars["name"])
					assert.Equal(t, tagName, vars["tagName"])
				}),
		)
	}
}

func StubFetchRefSHA(t *testing.T, reg *httpmock.Registry, owner, repoName, tagName, sha string) {
	path := fmt.Sprintf("repos/%s/%s/git/ref/tags%%2F%s", owner, repoName, tagName)
	reg.Register(
		httpmock.REST("GET", path),
		httpmock.StringResponse(fmt.Sprintf(`{"object": {"sha": "%s"}}`, sha)),
	)
}
