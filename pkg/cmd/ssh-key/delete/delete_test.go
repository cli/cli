package delete

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/ssh-key/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		input      string
		output     DeleteOptions
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:   "tty",
			tty:    true,
			input:  "123",
			output: DeleteOptions{KeyID: "123", Confirmed: false},
		},
		{
			name:   "confirm flag tty",
			tty:    true,
			input:  "123 --yes",
			output: DeleteOptions{KeyID: "123", Confirmed: true},
		},
		{
			name:   "shorthand confirm flag tty",
			tty:    true,
			input:  "123 -y",
			output: DeleteOptions{KeyID: "123", Confirmed: true},
		},
		{
			name:       "no tty",
			input:      "123",
			wantErr:    true,
			wantErrMsg: "--yes required when not running interactively",
		},
		{
			name:   "confirm flag no tty",
			input:  "123 --yes",
			output: DeleteOptions{KeyID: "123", Confirmed: true},
		},
		{
			name:   "shorthand confirm flag no tty",
			input:  "123 -y",
			output: DeleteOptions{KeyID: "123", Confirmed: true},
		},
		{
			name:       "no args",
			input:      "",
			wantErr:    true,
			wantErrMsg: "cannot delete: key id required",
		},
		{
			name:       "too many args",
			input:      "123 456",
			wantErr:    true,
			wantErrMsg: "too many arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

			var cmdOpts *DeleteOptions
			cmd := NewCmdDelete(f, func(opts *DeleteOptions) error {
				cmdOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.Error(t, err)
				assert.EqualError(t, err, tt.wantErrMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.output.KeyID, cmdOpts.KeyID)
			assert.Equal(t, tt.output.Confirmed, cmdOpts.Confirmed)
		})
	}
}

func Test_deleteRun(t *testing.T) {
	keyResp := "{\"title\":\"My Key\"}"
	signingKeyResp := "{\"title\":\"My Signing Key\"}"
	tests := []struct {
		name          string
		tty           bool
		opts          DeleteOptions
		httpStubs     func(*httpmock.Registry)
		prompterStubs func(*prompter.PrompterMock)
		wantStdout    string
		wantErr       bool
		wantErrMsg    string
	}{
		{
			name: "delete tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "123"},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.ConfirmDeletionFunc = func(_ string) error {
					return nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(200, keyResp))
				reg.Register(httpmock.REST("DELETE", "user/keys/123"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "✓ SSH key \"My Key\" (123) deleted from your account\n",
		},
		{
			name: "delete with confirm flag tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "123", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(200, keyResp))
				reg.Register(httpmock.REST("DELETE", "user/keys/123"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "✓ SSH key \"My Key\" (123) deleted from your account\n",
		},
		{
			name: "delete signing key tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "123"},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.ConfirmDeletionFunc = func(_ string) error {
					return nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(404, ""))
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(200, signingKeyResp))
				reg.Register(httpmock.REST("DELETE", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "✓ SSH key \"My Signing Key\" (123) deleted from your account\n",
		},
		{
			name: "delete signing key no tty",
			opts: DeleteOptions{KeyID: "123", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(404, ""))
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(200, signingKeyResp))
				reg.Register(httpmock.REST("DELETE", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "",
		},
		{
			name: "not found tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "123"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(404, ""))
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(404, `{"message":"Not Found"}`))
			},
			wantErr:    true,
			wantErrMsg: "HTTP 404 (https://api.github.com/user/ssh_signing_keys/123)",
		},
		{
			name: "delete no tty",
			opts: DeleteOptions{KeyID: "123", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(200, keyResp))
				reg.Register(httpmock.REST("DELETE", "user/keys/123"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "",
		},
		{
			name: "not found no tty",
			opts: DeleteOptions{KeyID: "123", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(404, ""))
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(404, `{"message":"Not Found"}`))
			},
			wantErr:    true,
			wantErrMsg: "HTTP 404 (https://api.github.com/user/ssh_signing_keys/123)",
		},
	}

	for _, tt := range tests {
		pm := &prompter.PrompterMock{}
		if tt.prompterStubs != nil {
			tt.prompterStubs(pm)
		}
		tt.opts.Prompter = pm

		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}

		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		tt.opts.Config = func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		}
		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdinTTY(tt.tty)
		ios.SetStdoutTTY(tt.tty)
		tt.opts.IO = ios

		t.Run(tt.name, func(t *testing.T) {
			err := deleteRun(&tt.opts)
			reg.Verify(t)
			if tt.wantErr {
				assert.Error(t, err)
				assert.EqualError(t, err, tt.wantErrMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}

func TestDeleteSSHKeyHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("DELETE", "user/keys/1234"),
		httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
	)

	err := deleteSSHKey(&http.Client{Transport: reg}, "github.com", "1234", shared.AuthenticationKey)

	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestDeleteSSHKeySigningKey(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("DELETE", "user/ssh_signing_keys/5678"),
		httpmock.StatusStringResponse(http.StatusNoContent, ""),
	)

	err := deleteSSHKey(&http.Client{Transport: reg}, "github.com", "5678", shared.SigningKey)

	require.NoError(t, err)
}

func TestGetSSHKeyAuthenticationKey(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "user/keys/5678"),
		httpmock.StatusStringResponse(200, `{"title":"My Key"}`),
	)
	// A signing-key lookup must not happen when the authentication key is found.
	reg.Exclude(t, httpmock.REST("GET", "user/ssh_signing_keys/5678"))

	key, keyType, err := getSSHKey(&http.Client{Transport: reg}, "github.com", "5678")

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, "My Key", key.Title)
	assert.Equal(t, shared.AuthenticationKey, keyType)
}

func TestGetSSHKeySigningKeyFallback(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "user/keys/5678"),
		httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
	)
	reg.Register(
		httpmock.REST("GET", "user/ssh_signing_keys/5678"),
		httpmock.StatusStringResponse(200, `{"title":"My Signing Key"}`),
	)

	key, keyType, err := getSSHKey(&http.Client{Transport: reg}, "github.com", "5678")

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, "My Signing Key", key.Title)
	assert.Equal(t, shared.SigningKey, keyType)
}

func TestGetSSHKeyAuthenticationEndpointErrorNotRetried(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "user/keys/5678"),
		httpmock.StatusStringResponse(http.StatusForbidden, `{"message":"Forbidden"}`),
	)
	// A non-404 error from the authentication endpoint must not trigger a
	// signing-key lookup.
	reg.Exclude(t, httpmock.REST("GET", "user/ssh_signing_keys/5678"))

	_, _, err := getSSHKey(&http.Client{Transport: reg}, "github.com", "5678")

	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusForbidden, httpErr.StatusCode)
}

func TestGetSSHKeyBothEndpointsNotFound(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "user/keys/1234"),
		httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
	)
	reg.Register(
		httpmock.REST("GET", "user/ssh_signing_keys/1234"),
		httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
	)

	key, _, err := getSSHKey(&http.Client{Transport: reg}, "github.com", "1234")

	assert.Nil(t, key)
	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "HTTP 404")
}
