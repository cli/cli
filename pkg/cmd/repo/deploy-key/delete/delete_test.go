package delete

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func Test_deleteRun(t *testing.T) {
	io, stdin, stdout, stderr := iostreams.Test()
	io.SetStdinTTY(false)
	io.SetStdoutTTY(true)
	io.SetStderrTTY(true)

	stdin.WriteString("PUBKEY")

	tr := httpmock.Registry{}
	defer tr.Verify(t)

	tr.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/keys/1234"),
		httpmock.StringResponse(`{}`))

	tr.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/keys/1234"),
		httpmock.StringResponse(`{}`))

	err := deleteRun(&DeleteOptions{
		IO: io,
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		HTTPClient: func() (*http.Client, error) {
			return &http.Client{Transport: &tr}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		ID: "1234",
	})
	assert.NoError(t, err)

	assert.Equal(t, "", stderr.String())
	assert.Equal(t, "✓ Public key deleted from your repository\n", stdout.String())
}
