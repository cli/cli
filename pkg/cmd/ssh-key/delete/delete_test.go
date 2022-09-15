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
			input:  "123",
			output: DeleteOptions{KeyID: "123", Confirmed: false},
		},
		{
			name:   "confirm flag tty",
			tty:    true,
			input:  "123 --confirm",
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
			wantErrMsg: "--confirm required when not running interactively",
		},
		{
			name:   "confirm flag no tty",
			input:  "123 --confirm",
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
			name: "prompting confirmation delete tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "123"},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.ConfirmFunc = func(s string, b bool) (bool, error) {
					return true, nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("DELETE", "user/keys/123"), httpmock.StatusStringResponse(204, "{}"))
			},
			wantErr:    false,
			wantStdout: "✓ Deleted SSH key id 123 from your account\n",
			wantErrMsg: "",
		},
		{
			name: "cancel delete tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "123"},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.ConfirmFunc = func(s string, b bool) (bool, error) {
					return false, nil
				}
			},
			wantErr:    true,
			wantStdout: "",
			wantErrMsg: "CancelError",
		},
		{
			name: "delete with confirm flag tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "123", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("DELETE", "user/keys/123"), httpmock.StatusStringResponse(204, "{}"))
			},
			wantErr:    false,
			wantStdout: "✓ Deleted SSH key id 123 from your account\n",
			wantErrMsg: "",
		},
		{
			name: "not found tty",
			tty:  true,
			opts: DeleteOptions{KeyID: "123"},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.ConfirmFunc = func(s string, b bool) (bool, error) {
					return true, nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("DELETE", "user/keys/123"), httpmock.StatusStringResponse(404, "{}"))
			},
			wantErr:    true,
			wantStdout: "",
			wantErrMsg: "unable to delete SSH key id 123: either the SSH key is not found or it is not owned by you",
		},
		{
			name: "delete no tty",
			opts: DeleteOptions{KeyID: "123"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("DELETE", "user/keys/123"), httpmock.StatusStringResponse(204, "{}"))
			},
			wantErr:    false,
			wantStdout: "",
			wantErrMsg: "",
		},
		{
			name: "not found no tty",
			opts: DeleteOptions{KeyID: "123"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("DELETE", "user/keys/123"), httpmock.StatusStringResponse(404, "{}"))
			},
			wantErr:    true,
			wantStdout: "",
			wantErrMsg: "unable to delete SSH key id 123: either the SSH key is not found or it is not owned by you",
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
