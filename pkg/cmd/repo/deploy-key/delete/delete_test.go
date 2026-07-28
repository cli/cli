package delete

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
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

	err := deleteRun(&DeleteOptions{
		IO: ios,
		HTTPClient: func() (*http.Client, error) {
			return &http.Client{Transport: &tr}, nil
		},
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

	err := deleteDeployKey(
		&http.Client{Transport: reg},
		ghrepo.New("OWNER", "REPO"),
		"1234",
	)

	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "HTTP 404")
}
