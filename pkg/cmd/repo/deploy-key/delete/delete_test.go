package delete

import (
	"context"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_deleteRun(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdinTTY(false)
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)

	tr := httpmock.Registry{}
	defer tr.Verify(t)

	tr.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/keys/1234"),
		httpmock.StringResponse(`{}`))

	err := deleteRun(context.Background(), &DeleteOptions{
		IO:         ios,
		GitHubREST: httpmock.RESTClientFunc(&tr),
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		KeyID: "1234",
	})
	assert.NoError(t, err)

	assert.Equal(t, "", stderr.String())
	assert.Equal(t, "✓ Deploy key deleted from OWNER/REPO\n", stdout.String())
}

func TestDeleteDeployKeyHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/keys/1234"),
		httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
	)

	client, err := httpmock.RESTClientFunc(reg)("github.com")
	require.NoError(t, err)

	err = deleteDeployKey(
		context.Background(),
		client,
		ghrepo.New("OWNER", "REPO"),
		"1234",
	)

	var httpErr *githubrest.ErrorResponse
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "HTTP 404")
}
