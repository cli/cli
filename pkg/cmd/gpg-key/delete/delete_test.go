package delete

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
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
			input:  "ABC123",
			output: DeleteOptions{KeyID: "ABC123", Confirmed: false},
		},
		{
			name:   "confirm flag tty",
			tty:    true,
			input:  "ABC123 --yes",
			output: DeleteOptions{KeyID: "ABC123", Confirmed: true},
		},
		{
			name:   "shorthand confirm flag tty",
			tty:    true,
			input:  "ABC123 -y",
			output: DeleteOptions{KeyID: "ABC123", Confirmed: true},
		},
		{
			name:       "no tty",
			input:      "ABC123",
			wantErr:    true,
			wantErrMsg: "--yes required when not running interactively",
		},
		{
			name:   "confirm flag no tty",
			input:  "ABC123 --yes",
			output: DeleteOptions{KeyID: "ABC123", Confirmed: true},
		},
		{
			name:   "shorthand confirm flag no tty",
			input:  "ABC123 -y",
			output: DeleteOptions{KeyID: "ABC123", Confirmed: true},
		},
		{
			name:       "no args",
			input:      "",
			wantErr:    true,
			wantErrMsg: "cannot delete: key id required",
		},
		{
			name:       "too many args",
			input:      "ABC123 XYZ",
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
	keysResp := "[{\"id\":123,\"key_id\":\"ABC123\"}]"
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
			opts: DeleteOptions{KeyID: "ABC123", Confirmed: false},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.ConfirmDeletionFunc = func(_ string) error {
					return nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/gpg_keys"), httpmock.StatusStringResponse(200, keysResp))
				reg.Register(httpmock.REST("DELETE", "user/gpg_keys/123"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "✓ GPG key ABC123 deleted from your account\n",
		},
		{
			name: "delete with confirm flag tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "ABC123", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/gpg_keys"), httpmock.StatusStringResponse(200, keysResp))
				reg.Register(httpmock.REST("DELETE", "user/gpg_keys/123"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "✓ GPG key ABC123 deleted from your account\n",
		},
		{
			name: "not found tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "ABC123", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/gpg_keys"), httpmock.StatusStringResponse(200, "[]"))
			},
			wantErr:    true,
			wantErrMsg: "unable to delete GPG key ABC123: either the GPG key is not found or it is not owned by you",
		},
		{
			name: "delete no tty",
			opts: DeleteOptions{KeyID: "ABC123", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/gpg_keys"), httpmock.StatusStringResponse(200, keysResp))
				reg.Register(httpmock.REST("DELETE", "user/gpg_keys/123"), httpmock.StatusStringResponse(204, ""))
			},
			wantStdout: "",
		},
		{
			name: "not found no tty",
			opts: DeleteOptions{KeyID: "ABC123", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user/gpg_keys"), httpmock.StatusStringResponse(200, "[]"))
			},
			wantErr:    true,
			wantErrMsg: "unable to delete GPG key ABC123: either the GPG key is not found or it is not owned by you",
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
		tt.opts.Config = func() (config.Config, error) {
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
