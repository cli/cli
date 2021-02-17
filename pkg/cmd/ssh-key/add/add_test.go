package add

import (
	"net/http"
	"testing"

	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func Test_runAdd(t *testing.T) {
	io, stdin, stdout, stderr := iostreams.Test()
	io.SetStdinTTY(false)
	io.SetStdoutTTY(true)
	io.SetStderrTTY(true)

	stdin.WriteString("PUBKEY")

	tr := httpmock.Registry{}
	defer tr.Verify(t)

	tr.Register(
		httpmock.REST("POST", "user/keys"),
		httpmock.StringResponse(`{}`))

	err := runAdd(&AddOptions{
		IO: io,
		HTTPClient: func() (*http.Client, error) {
			return &http.Client{Transport: &tr}, nil
		},
		KeyFile: "-",
		Title:   "my sacred key",
	})
	assert.NoError(t, err)

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "✓ Public key added to your account\n", stderr.String())
}
