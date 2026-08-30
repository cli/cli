package add

import (
	"net/http"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_gpgKeyUploadScopesMissing(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("POST", "user/gpg_keys"),
		httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
	)

	err := gpgKeyUpload(&http.Client{Transport: reg}, "github.com", strings.NewReader("-----BEGIN PGP PUBLIC KEY BLOCK-----"), "")

	require.Same(t, errScopesMissing, err)
}

func Test_gpgKeyUploadDuplicateKeyBeforeWrongFormat(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("POST", "user/gpg_keys"),
		httpmock.WithHeader(httpmock.StatusStringResponse(http.StatusUnprocessableEntity, `{
			"message": "Validation Failed",
			"errors": [{
				"resource": "GpgKey",
				"code": "custom",
				"field": "key_id",
				"message": "key_id already exists"
			}]
		}`), "Content-Type", "application/json"),
	)

	err := gpgKeyUpload(&http.Client{Transport: reg}, "github.com", strings.NewReader("binary-key"), "")

	require.Same(t, errDuplicateKey, err)
}

func Test_gpgKeyUploadWrongFormat(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("POST", "user/gpg_keys"),
		httpmock.StatusStringResponse(http.StatusUnprocessableEntity, `{
			"message": "Validation Failed",
			"errors": [{
				"resource": "GpgKey",
				"code": "custom",
				"message": "We got an error doing that."
			}]
		}`),
	)

	err := gpgKeyUpload(&http.Client{Transport: reg}, "github.com", strings.NewReader("binary-key"), "")

	require.Same(t, errWrongFormat, err)
}

func Test_gpgKeyUploadHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("POST", "user/gpg_keys"),
		httpmock.WithHeader(
			httpmock.StatusStringResponse(http.StatusInternalServerError, `{"message":"Internal Server Error"}`),
			"Content-Type", "application/json",
		),
	)

	err := gpgKeyUpload(&http.Client{Transport: reg}, "github.com", strings.NewReader("-----BEGIN PGP PUBLIC KEY BLOCK-----"), "")

	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusInternalServerError, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "HTTP 500")
	assert.EqualError(t, err, "HTTP 500: Internal Server Error (https://api.github.com/user/gpg_keys)")
}

func Test_runAdd(t *testing.T) {
	tests := []struct {
		name       string
		stdin      string
		httpStubs  func(*httpmock.Registry)
		wantStdout string
		wantStderr string
		wantErrMsg string
		opts       AddOptions
	}{
		{
			name:  "valid key",
			stdin: "-----BEGIN PGP PUBLIC KEY BLOCK-----",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "user/gpg_keys"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]any) {
						assert.Contains(t, payload, "armored_public_key")
						assert.NotContains(t, payload, "title")
					}))
			},
			wantStdout: "✓ GPG key added to your account\n",
			wantStderr: "",
			wantErrMsg: "",
			opts:       AddOptions{KeyFile: "-"},
		},
		{
			name:  "valid key with title",
			stdin: "-----BEGIN PGP PUBLIC KEY BLOCK-----",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "user/gpg_keys"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]any) {
						assert.Contains(t, payload, "armored_public_key")
						assert.Contains(t, payload, "name")
					}))
			},
			wantStdout: "✓ GPG key added to your account\n",
			wantStderr: "",
			wantErrMsg: "",
			opts:       AddOptions{KeyFile: "-", Title: "some title"},
		},
		{
			name:  "binary format fails",
			stdin: "gCAAAAA7H7MHTZWFLJKD3vP4F7v",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "user/gpg_keys"),
					httpmock.StatusStringResponse(422, `{
						"message": "Validation Failed",
						"errors": [{
							"resource": "GpgKey",
							"code": "custom",
							"message": "We got an error doing that."
						}],
						"documentation_url": "https://docs.github.com/v3/users/gpg_keys"
					}`),
				)
			},
			wantStdout: "",
			wantStderr: heredoc.Doc(`
				X Error: the GPG key you are trying to upload might not be in ASCII-armored format.
				Find your GPG key ID with:    gpg --list-keys
				Then add it to your account:  gpg --armor --export <ID> | gh gpg-key add -
			`),
			wantErrMsg: "SilentError",
			opts:       AddOptions{KeyFile: "-"},
		},
		{
			name:  "duplicate key",
			stdin: "-----BEGIN PGP PUBLIC KEY BLOCK-----",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "user/gpg_keys"),
					httpmock.WithHeader(httpmock.StatusStringResponse(422, `{
						"message": "Validation Failed",
						"errors": [{
							"resource": "GpgKey",
							"code": "custom",
							"field": "key_id",
							"message": "key_id already exists"
						}, {
							"resource": "GpgKey",
							"code": "custom",
							"field": "public_key",
							"message": "public_key already exists"
						}],
						"documentation_url": "https://docs.github.com/v3/users/gpg_keys"
					}`), "Content-type", "application/json"),
				)
			},
			wantStdout: "",
			wantStderr: "X Error: the key already exists in your account\n",
			wantErrMsg: "SilentError",
			opts:       AddOptions{KeyFile: "-"},
		},
	}

	for _, tt := range tests {
		ios, stdin, stdout, stderr := iostreams.Test()
		ios.SetStdinTTY(true)
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
