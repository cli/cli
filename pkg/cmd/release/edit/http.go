package edit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/shurcooL/githubv4"
)

// responseTracker is a http.RoundTripper that records the status code of each
// response and wraps the body to catch non-EOF read errors. editRelease uses it
// to restore the phase-dependent return shape (nil pointer for pre-decode errors,
// non-nil for any post-response outcome) without bypassing the shared REST client.
type responseTracker struct {
	transport   http.RoundTripper
	statusCode  int
	bodyReadErr error
}

func (t *responseTracker) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.transport.RoundTrip(req)
	if resp != nil {
		t.statusCode = resp.StatusCode
		t.bodyReadErr = nil
		if resp.Body != nil {
			resp.Body = &bodyReadTracker{ReadCloser: resp.Body, tracker: t}
		}
	}
	return resp, err
}

// bodyReadTracker wraps a response body and records the most recent non-EOF read error.
type bodyReadTracker struct {
	io.ReadCloser
	tracker *responseTracker
}

func (b *bodyReadTracker) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && err != io.EOF {
		b.tracker.bodyReadErr = err
	}
	return n, err
}

func editRelease(httpClient *http.Client, repo ghrepo.Interface, releaseID int64, params map[string]interface{}) (*shared.Release, error) {
	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases", strconv.FormatInt(releaseID, 10))
	if err != nil {
		return nil, err
	}

	transport := httpClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	tracker := &responseTracker{transport: transport}
	clone := *httpClient
	clone.Transport = tracker

	newRelease := &shared.Release{}
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(&clone).REST(repo.RepoHost(), "PATCH", path.String(), bytes.NewReader(bodyBytes), newRelease)
	// Return nil only when there is an actual error AND no 2xx response was decoded.
	// status 0 + nil error (impossible in practice) falls through to non-nil release.
	if err != nil && (tracker.statusCode == 0 || tracker.statusCode < 200 || tracker.statusCode >= 300 || tracker.bodyReadErr != nil) {
		return nil, err
	}
	if tracker.statusCode == http.StatusNoContent || tracker.statusCode == http.StatusResetContent {
		return newRelease, json.Unmarshal(nil, newRelease)
	}
	return newRelease, err
}

func remoteTagExists(httpClient *http.Client, repo ghrepo.Interface, tagName string) (bool, error) {
	gql := api.NewClientFromHTTP(httpClient)
	qualifiedTagName := fmt.Sprintf("refs/tags/%s", tagName)
	var query struct {
		Repository struct {
			Ref struct {
				ID string
			} `graphql:"ref(qualifiedName: $tagName)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	variables := map[string]interface{}{
		"owner":   githubv4.String(repo.RepoOwner()),
		"name":    githubv4.String(repo.RepoName()),
		"tagName": githubv4.String(qualifiedTagName),
	}
	err := gql.Query(repo.RepoHost(), "RepositoryFindRef", &query, variables)
	return query.Repository.Ref.ID != "", err
}
