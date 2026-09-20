package add

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_runAdd(t *testing.T) {
	tests := []struct {
		name       string
		stdin      string
		opts       AddOptions
		httpStubs  func(*httpmock.Registry)
		wantStdout string
		wantStderr string
		wantErrMsg string
	}{
		{
			name:  "valid key format, not already in use",
			stdin: "ssh-ed25519 asdf",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user/keys"),
					httpmock.StringResponse("[]"))
				reg.Register(
					httpmock.REST("POST", "user/keys"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]any) {
						assert.Contains(t, payload, "key")
						assert.Empty(t, payload["title"])
					}))
			},
			wantStdout: "",
			wantStderr: "✓ Public key added to your account\n",
			wantErrMsg: "",
			opts:       AddOptions{KeyFile: "-"},
		},
		{
			name:  "valid signing key format, not already in use",
			stdin: "ssh-ed25519 asdf",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user/ssh_signing_keys"),
					httpmock.StringResponse("[]"))
				reg.Register(
					httpmock.REST("POST", "user/ssh_signing_keys"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]any) {
						assert.Contains(t, payload, "key")
						assert.Empty(t, payload["title"])
					}))
			},
			wantStdout: "",
			wantStderr: "✓ Public key added to your account\n",
			wantErrMsg: "",
			opts:       AddOptions{KeyFile: "-", Type: "signing"},
		},
		{
			name:  "valid key format, already in use",
			stdin: "ssh-ed25519 asdf title",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user/keys"),
					httpmock.StringResponse(`[
						{
							"id": 1,
							"key": "ssh-ed25519 asdf",
							"title": "anything"
						}
					]`))
			},
			wantStdout: "",
			wantStderr: "✓ Public key already exists on your account\n",
			wantErrMsg: "",
			opts:       AddOptions{KeyFile: "-"},
		},
		{
			name:  "valid signing key format, already in use",
			stdin: "ssh-ed25519 asdf title",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user/ssh_signing_keys"),
					httpmock.StringResponse(`[
						{
							"id": 1,
							"key": "ssh-ed25519 asdf",
							"title": "anything"
						}
					]`))
			},
			wantStdout: "",
			wantStderr: "✓ Public key already exists on your account\n",
			wantErrMsg: "",
			opts:       AddOptions{KeyFile: "-", Type: "signing"},
		},
		{
			name:       "invalid key format",
			stdin:      "ssh-ed25519",
			wantStdout: "",
			wantStderr: "",
			wantErrMsg: "provided key is not in a valid format",
			opts:       AddOptions{KeyFile: "-"},
		},
		{
			name:       "invalid signing key format",
			stdin:      "ssh-ed25519",
			wantStdout: "",
			wantStderr: "",
			wantErrMsg: "provided key is not in a valid format",
			opts:       AddOptions{KeyFile: "-", Type: "signing"},
		},
	}

	for _, tt := range tests {
		ios, stdin, stdout, stderr := iostreams.Test()
		ios.SetStdinTTY(false)
		ios.SetStdoutTTY(true)
		ios.SetStderrTTY(true)

		stdin.WriteString(tt.stdin)

		reg := &httpmock.Registry{}

		tt.opts.IO = ios
		tt.opts.HTTPClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.Config = func() (gh.Config, error) {
			return config.NewMockConfig(), nil
		}

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			err := runAdd(&tt.opts)
			if tt.wantErrMsg != "" {
				assert.Equal(t, tt.wantErrMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

func TestSSHKeyUploadHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "user/keys"),
		httpmock.StringResponse("[]"),
	)
	reg.Register(
		httpmock.REST("POST", "user/keys"),
		httpmock.StatusStringResponse(http.StatusUnprocessableEntity, `{"message":"Validation Failed"}`),
	)

	uploaded, err := SSHKeyUpload(
		&http.Client{Transport: reg},
		"github.com",
		strings.NewReader("ssh-ed25519 asdf"),
		"",
	)

	assert.False(t, uploaded)
	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnprocessableEntity, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "HTTP 422")
}
