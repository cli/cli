package create

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTagsHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/tags"),
		httpmock.StatusStringResponse(http.StatusInternalServerError, `{"message":"Internal Server Error"}`),
	)

	_, err := getTags(&http.Client{Transport: reg}, ghrepo.New("OWNER", "REPO"), 5)

	requireAPIHTTPError(t, err, http.StatusInternalServerError)
}

func TestGenerateReleaseNotesNotImplemented(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("POST", "repos/OWNER/REPO/releases/generate-notes"),
		httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
	)

	_, err := generateReleaseNotes(&http.Client{Transport: reg}, ghrepo.New("OWNER", "REPO"), "v1.2.3", "", "")

	assert.Same(t, notImplementedError, err)
}

func TestGenerateReleaseNotesHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("POST", "repos/OWNER/REPO/releases/generate-notes"),
		httpmock.StatusStringResponse(http.StatusInternalServerError, `{"message":"Internal Server Error"}`),
	)

	_, err := generateReleaseNotes(&http.Client{Transport: reg}, ghrepo.New("OWNER", "REPO"), "v1.2.3", "", "")

	requireAPIHTTPError(t, err, http.StatusInternalServerError)
}

func TestCreateReleaseMissingWorkflowScope(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("POST", "repos/OWNER/REPO/releases"),
		httpmock.StatusScopesResponder(http.StatusNotFound, "repo,read:org"),
	)

	_, err := createRelease(&http.Client{Transport: reg}, ghrepo.New("OWNER", "REPO"), map[string]any{"tag_name": "v1.2.3"})

	var scopeErr *errMissingRequiredWorkflowScope
	require.ErrorAs(t, err, &scopeErr)
	assert.Equal(t, "github.com", scopeErr.Hostname)
	assert.EqualError(t, err, "workflow scope may be required")
}

func TestCreateReleaseHTTPErrorWithoutScopesHeader(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("POST", "repos/OWNER/REPO/releases"),
		httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
	)

	_, err := createRelease(&http.Client{Transport: reg}, ghrepo.New("OWNER", "REPO"), map[string]any{"tag_name": "v1.2.3"})

	requireAPIHTTPError(t, err, http.StatusNotFound)
}

func TestPublishReleaseHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("PATCH", "releases/123"),
		httpmock.StatusStringResponse(http.StatusInternalServerError, `{"message":"Internal Server Error"}`),
	)

	_, err := publishRelease(&http.Client{Transport: reg}, "github.com", safeurl.NewImmutableSafeURL("https://api.github.com/releases/123"), "", nil)

	requireAPIHTTPError(t, err, http.StatusInternalServerError)
}

func TestDeleteReleaseHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("DELETE", "releases/123"),
		httpmock.StatusStringResponse(http.StatusInternalServerError, `{"message":"Internal Server Error"}`),
	)

	err := deleteRelease(&http.Client{Transport: reg}, "github.com", safeurl.NewImmutableSafeURL("https://api.github.com/releases/123"))

	requireAPIHTTPError(t, err, http.StatusInternalServerError)
}

func TestTokenHasWorkflowScope(t *testing.T) {
	tests := []struct {
		name   string
		scopes string
		want   bool
	}{
		{
			// The API separates scopes with a comma and a space, and lists them
			// in sorted order, so workflow is almost never the first element.
			name:   "realistically spaced scopes including workflow",
			scopes: "gist, project, read:org, repo, user, workflow",
			want:   true,
		},
		{
			name:   "spaced scopes without workflow",
			scopes: "repo, read:org",
			want:   false,
		},
		{
			name:   "workflow first",
			scopes: "workflow, repo",
			want:   true,
		},
		{
			name:   "only workflow",
			scopes: "workflow",
			want:   true,
		},
		{
			name:   "unspaced scopes including workflow",
			scopes: "repo,read:org,workflow",
			want:   true,
		},
		{
			// An absent header means this is not an OAuth token, so we can't
			// know its scopes and must assume workflow is present.
			name:   "no scopes header",
			scopes: "",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			if tt.scopes != "" {
				headers.Set("X-Oauth-Scopes", tt.scopes)
			}

			assert.Equal(t, tt.want, tokenHasWorkflowScope(headers))
		})
	}
}

func requireAPIHTTPError(t *testing.T, err error, statusCode int) {
	t.Helper()

	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, statusCode, httpErr.StatusCode)
	assert.Contains(t, err.Error(), fmt.Sprintf("HTTP %d", statusCode))
}
