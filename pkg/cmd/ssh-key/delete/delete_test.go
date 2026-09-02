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
			name:   "type authentication",
			tty:    true,
			input:  "123 --type authentication --yes",
			output: DeleteOptions{KeyID: "123", Type: shared.AuthenticationKey, Confirmed: true},
		},
		{
			name:   "type signing",
			tty:    true,
			input:  "123 --type signing --yes",
			output: DeleteOptions{KeyID: "123", Type: shared.SigningKey, Confirmed: true},
		},
		{
			name:       "invalid type",
			tty:        true,
			input:      "123 --type bogus --yes",
			wantErr:    true,
			wantErrMsg: "invalid argument \"bogus\" for \"--type\" flag: valid values are {authentication|signing}",
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
			assert.Equal(t, tt.output.Type, cmdOpts.Type)
		})
	}
}

func Test_deleteRun(t *testing.T) {
	authKeyResp := `{"title":"My Auth Key"}`
	signingKeyResp := `{"title":"My Signing Key"}`
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
			name: "delete authentication key tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "123"},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.ConfirmDeletionFunc = func(_ string) error {
					return nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(200, authKeyResp))
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(404, ""))
				reg.Register(httpmock.REST("DELETE", "user/keys/123"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "✓ SSH key \"My Auth Key\" (123) deleted from your account\n",
		},
		{
			name: "delete signing key tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "456"},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.ConfirmDeletionFunc = func(prompt string) error {
					assert.Equal(t, "My Signing Key (signing)", prompt)
					return nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/456"), httpmock.StatusStringResponse(404, ""))
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/456"), httpmock.StatusStringResponse(200, signingKeyResp))
				reg.Register(httpmock.REST("DELETE", "user/ssh_signing_keys/456"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "✓ SSH key \"My Signing Key\" (456) deleted from your account\n",
		},
		{
			name: "delete with confirm flag tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "123", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(200, authKeyResp))
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(404, ""))
				reg.Register(httpmock.REST("DELETE", "user/keys/123"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "✓ SSH key \"My Auth Key\" (123) deleted from your account\n",
		},
		{
			name: "delete signing key with --type",
			tty:  true,
			opts: DeleteOptions{KeyID: "456", Type: shared.SigningKey, Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/456"), httpmock.StatusStringResponse(200, signingKeyResp))
				reg.Register(httpmock.REST("DELETE", "user/ssh_signing_keys/456"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "✓ SSH key \"My Signing Key\" (456) deleted from your account\n",
		},
		{
			name: "ambiguous id prompts for type tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "123"},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(prompt, _ string, options []string) (int, error) {
					assert.Contains(t, prompt, "123")
					assert.Equal(t, []string{
						"My Auth Key (authentication)",
						"My Signing Key (signing)",
					}, options)
					return 1, nil
				}
				pm.ConfirmDeletionFunc = func(_ string) error {
					return nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(200, authKeyResp))
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(200, signingKeyResp))
				reg.Register(httpmock.REST("DELETE", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "✓ SSH key \"My Signing Key\" (123) deleted from your account\n",
		},
		{
			name: "ambiguous id without tty requires --type",
			opts: DeleteOptions{KeyID: "123", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(200, authKeyResp))
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(200, signingKeyResp))
			},
			wantErr: true,
			wantErrMsg: "SSH key ID 123 matches both an authentication key (\"My Auth Key\") and a signing key (\"My Signing Key\"); " +
				"re-run with --type authentication or --type signing",
		},
		{
			name: "not found tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "123"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(404, ""))
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(404, ""))
			},
			wantErr:    true,
			wantErrMsg: "SSH key not found: 123",
		},
		{
			name: "delete authentication key no tty",
			opts: DeleteOptions{KeyID: "123", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(200, authKeyResp))
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(404, ""))
				reg.Register(httpmock.REST("DELETE", "user/keys/123"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "",
		},
		{
			name: "delete signing key no tty",
			opts: DeleteOptions{KeyID: "456", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/456"), httpmock.StatusStringResponse(404, ""))
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/456"), httpmock.StatusStringResponse(200, signingKeyResp))
				reg.Register(httpmock.REST("DELETE", "user/ssh_signing_keys/456"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "",
		},
		{
			name: "not found no tty",
			opts: DeleteOptions{KeyID: "123", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/keys/123"), httpmock.StatusStringResponse(404, ""))
				reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/123"), httpmock.StatusStringResponse(404, ""))
			},
			wantErr:    true,
			wantErrMsg: "SSH key not found: 123",
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
			return config.NewMockConfig(), nil
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

func TestDeleteSigningSSHKeyHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("DELETE", "user/ssh_signing_keys/1234"),
		httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
	)

	err := deleteSSHKey(&http.Client{Transport: reg}, "github.com", "1234", shared.SigningKey)

	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestGetSSHKeyHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "user/keys/1234"),
		httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
	)

	key, err := getSSHKey(&http.Client{Transport: reg}, "github.com", "1234", shared.AuthenticationKey)

	assert.Nil(t, key)
	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestResolveSSHKey(t *testing.T) {
	t.Run("authentication only", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.REST("GET", "user/keys/1"), httpmock.StatusStringResponse(200, `{"title":"auth"}`))
		reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/1"), httpmock.StatusStringResponse(404, ""))

		key, err := resolveSSHKey(&http.Client{Transport: reg}, "github.com", "1", "")
		require.NoError(t, err)
		assert.Equal(t, "auth", key.Title)
		assert.Equal(t, shared.AuthenticationKey, key.Type)
	})

	t.Run("signing only", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.REST("GET", "user/keys/2"), httpmock.StatusStringResponse(404, ""))
		reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/2"), httpmock.StatusStringResponse(200, `{"title":"sign"}`))

		key, err := resolveSSHKey(&http.Client{Transport: reg}, "github.com", "2", "")
		require.NoError(t, err)
		assert.Equal(t, "sign", key.Title)
		assert.Equal(t, shared.SigningKey, key.Type)
	})

	t.Run("explicit signing type", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/3"), httpmock.StatusStringResponse(200, `{"title":"sign"}`))

		key, err := resolveSSHKey(&http.Client{Transport: reg}, "github.com", "3", shared.SigningKey)
		require.NoError(t, err)
		assert.Equal(t, shared.SigningKey, key.Type)
	})

	t.Run("ambiguous", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.REST("GET", "user/keys/4"), httpmock.StatusStringResponse(200, `{"title":"auth"}`))
		reg.Register(httpmock.REST("GET", "user/ssh_signing_keys/4"), httpmock.StatusStringResponse(200, `{"title":"sign"}`))

		key, err := resolveSSHKey(&http.Client{Transport: reg}, "github.com", "4", "")
		assert.Nil(t, key)
		var ambiguous ambiguousKeyError
		require.ErrorAs(t, err, &ambiguous)
		assert.Equal(t, "4", ambiguous.KeyID)
	})
}
