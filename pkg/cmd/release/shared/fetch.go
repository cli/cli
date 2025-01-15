package shared

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
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
	State  string
	APIURL string `json:"url"`

	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	DownloadCount      int       `json:"download_count"`
	ContentType        string    `json:"content_type"`
	BrowserDownloadURL string    `json:"browser_download_url"`
}

func (rel *Release) ExportData(fields []string) map[string]interface{} {
	v := reflect.ValueOf(rel).Elem()
	fieldByName := func(v reflect.Value, field string) reflect.Value {
		return v.FieldByNameFunc(func(s string) bool {
			return strings.EqualFold(field, s)
		})
	}
	data := map[string]interface{}{}

	for _, f := range fields {
		switch f {
		case "author":
			data[f] = map[string]interface{}{
				"id":    rel.Author.ID,
				"login": rel.Author.Login,
			}
		case "assets":
			assets := make([]interface{}, 0, len(rel.Assets))
			for _, a := range rel.Assets {
				assets = append(assets, map[string]interface{}{
					"url":           a.BrowserDownloadURL,
					"apiUrl":        a.APIURL,
					"id":            a.ID,
					"name":          a.Name,
					"label":         a.Label,
					"size":          a.Size,
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

var errNotFound = errors.New("release not found")

type fetchResult struct {
	release *Release
	error   error
}

// FetchRelease finds a published repository release by its tagName, or a draft release by its pending tag name.
func FetchRelease(ctx context.Context, httpClient *http.Client, repo ghrepo.Interface, tagName string) (*Release, error) {
	cc, cancel := context.WithCancel(ctx)
	results := make(chan fetchResult, 2)

	// published release lookup
	go func() {
		path := fmt.Sprintf("repos/%s/%s/releases/tags/%s", repo.RepoOwner(), repo.RepoName(), tagName)
		release, err := fetchReleasePath(cc, httpClient, repo.RepoHost(), path)
		results <- fetchResult{release: release, error: err}
	}()

	// draft release lookup
	go func() {
		release, err := fetchDraftRelease(cc, httpClient, repo, tagName)
		results <- fetchResult{release: release, error: err}
	}()

	res := <-results
	if errors.Is(res.error, errNotFound) {
		res = <-results
		cancel() // satisfy the linter even though no goroutines are running anymore
	} else {
		cancel()
		<-results // drain the channel
	}
	return res.release, res.error
}

// FetchLatestRelease finds the latest published release for a repository.
func FetchLatestRelease(ctx context.Context, httpClient *http.Client, repo ghrepo.Interface) (*Release, error) {
	path := fmt.Sprintf("repos/%s/%s/releases/latest", repo.RepoOwner(), repo.RepoName())
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

	variables := map[string]interface{}{
		"owner":   githubv4.String(repo.RepoOwner()),
		"name":    githubv4.String(repo.RepoName()),
		"tagName": githubv4.String(tagName),
	}

	gql := api.NewClientFromHTTP(httpClient)
	if err := gql.QueryWithContext(ctx, repo.RepoHost(), "RepositoryReleaseByTag", &query, variables); err != nil {
		return nil, err
	}

	if query.Repository.Release == nil || !query.Repository.Release.IsDraft {
		return nil, errNotFound
	}

	// Then, use REST to get information about the draft release. In theory, we could have fetched
	// all the necessary information via GraphQL, but REST is safer for backwards compatibility.
	path := fmt.Sprintf("repos/%s/%s/releases/%d", repo.RepoOwner(), repo.RepoName(), query.Repository.Release.DatabaseID)
	return fetchReleasePath(ctx, httpClient, repo.RepoHost(), path)
}

func fetchReleasePath(ctx context.Context, httpClient *http.Client, host string, p string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", ghinstance.RESTPrefix(host)+p, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, errNotFound
	} else if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
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
				func(q string, vars map[string]interface{}) {
					assert.Equal(t, owner, vars["owner"])
					assert.Equal(t, repoName, vars["name"])
					assert.Equal(t, tagName, vars["tagName"])
				}),
		)
	}
}
