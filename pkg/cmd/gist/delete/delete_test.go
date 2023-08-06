package delete

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants DeleteOptions
	}{
		{
			name: "valid selector",
			cli:  "123",
			wants: DeleteOptions{
				Selector: "123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)
			var gotOpts *DeleteOptions
			cmd := NewCmdDelete(f, func(opts *DeleteOptions) error {
				gotOpts = opts
				return nil
			})

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Selector, gotOpts.Selector)
		})
	}
}

func Test_deleteRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       DeleteOptions
		httpStubs  func(*httpmock.Registry)
		wantErr    bool
		wantStdout string
		wantStderr string
	}{
		{
			name: "successfully delete",
			opts: DeleteOptions{
				Selector: "1234",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("DELETE", "gists/1234"),
					httpmock.StatusStringResponse(200, "{}"))
			},
			wantErr:    false,
			wantStdout: "",
			wantStderr: "",
		},
		{
			name: "not found",
			opts: DeleteOptions{
				Selector: "1234",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("DELETE", "gists/1234"),
					httpmock.StatusStringResponse(404, "{}"))
			},
			wantErr:    true,
			wantStdout: "",
			wantStderr: "",
		},
	}

	for _, tt := range tests {
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
		ios, _, _, _ := iostreams.Test()
		ios.SetStdoutTTY(false)
		ios.SetStdinTTY(false)
		tt.opts.IO = ios

		t.Run(tt.name, func(t *testing.T) {
			err := deleteRun(&tt.opts)
			reg.Verify(t)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantStderr != "" {
					assert.EqualError(t, err, tt.wantStderr)
				}
				return
			}
			assert.NoError(t, err)

		})
	}
}
