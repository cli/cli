package garden

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCommits(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/commits"),
		httpmock.JSONResponse([]map[string]interface{}{
			{"sha": "abc123def456", "author": map[string]string{"login": "monalisa"}},
			{"sha": "def456abc123", "author": map[string]string{"login": ""}},
		}),
	)

	commits, err := getCommits(&http.Client{Transport: reg}, ghrepo.New("OWNER", "REPO"), 10)
	require.NoError(t, err)

	require.Len(t, commits, 2)
	// commits are reversed so that older ones come first
	assert.Equal(t, "def456abc123", commits[0].Sha)
	assert.Equal(t, "a mysterious stranger", commits[0].Handle)
	assert.Equal(t, "abc123def456", commits[1].Sha)
	assert.Equal(t, "monalisa", commits[1].Handle)
}

// TestGetCommitsHTTPError pins the error returned on a non-2xx response. This site used to
// return a bare "api call failed", and now surfaces the API's own message via api.HTTPError.
func TestGetCommitsHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/commits"),
		httpmock.StatusJSONResponse(404, map[string]string{"message": "Not Found"}),
	)

	_, err := getCommits(&http.Client{Transport: reg}, ghrepo.New("OWNER", "REPO"), 10)
	require.Error(t, err)

	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, 404, httpErr.StatusCode)
	assert.Equal(t, "Not Found", httpErr.Message)
}
