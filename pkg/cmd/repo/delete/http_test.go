package delete

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteRepo(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO"),
		httpmock.StatusStringResponse(204, ""),
	)

	require.NoError(t, deleteRepo(&http.Client{Transport: reg}, ghrepo.New("OWNER", "REPO")))
}

// TestDeleteRepoMissingScope pins the scopes suggestion, which is the reason this call site
// needs the delete_repo scope declared rather than relying on what the API reports.
func TestDeleteRepoMissingScope(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO"),
		func(req *http.Request) (*http.Response, error) {
			resp, err := httpmock.StatusJSONResponse(403, map[string]string{"message": "Must have admin rights to Repository."})(req)
			if err != nil {
				return nil, err
			}
			resp.Header.Set("X-Oauth-Scopes", "repo")
			resp.Header.Set("X-Accepted-Oauth-Scopes", "")
			return resp, nil
		},
	)

	err := deleteRepo(&http.Client{Transport: reg}, ghrepo.New("OWNER", "REPO"))
	require.Error(t, err)

	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Contains(t, httpErr.ScopesSuggestion(), "delete_repo")
}

// TestDeleteRepoDoesNotFollowRedirect pins that a renamed or transferred repo surfaces the 3xx
// to the caller rather than being followed. deleteRun relies on seeing that status to explain
// what happened, so following the redirect would report success while deleting nothing.
//
// The Location header matters: without it Go cannot follow a redirect at all, so a stub that
// omits it passes whatever the redirect policy is.
func TestDeleteRepoDoesNotFollowRedirect(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO"),
		func(req *http.Request) (*http.Response, error) {
			resp, err := httpmock.StatusStringResponse(301, "")(req)
			if err != nil {
				return nil, err
			}
			resp.Header.Set("Location", "https://api.github.com/repos/OWNER/RENAMED")
			return resp, nil
		},
	)

	err := deleteRepo(&http.Client{Transport: reg}, ghrepo.New("OWNER", "REPO"))
	require.Error(t, err)

	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, 301, httpErr.StatusCode)
}
