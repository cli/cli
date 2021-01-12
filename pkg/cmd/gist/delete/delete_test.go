package delete

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/pkg/cmd/gist/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
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
		opts       *DeleteOptions
		gist       *shared.Gist
		httpStubs  func(*httpmock.Registry)
		askStubs   func(*prompt.AskStubber)
		nontty     bool
		wantErr    bool
		wantStderr string
		wantParams map[string]interface{}
	}{
		{
			name:    "no such gist",
			wantErr: true,
		}, {
			name: "another user's gist",
			gist: &shared.Gist{
				ID: "1234",
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Filename: "cicada.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
						Type:     "text/plain",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat2"},
			},
			wantErr:    true,
			wantStderr: "You do not own this gist.",
		}, {
			name: "successfully delete",
			gist: &shared.Gist{
				ID: "1234",
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Filename: "cicada.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
						Type:     "text/plain",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("DELETE", "gists/1234"),
					httpmock.StringResponse("{}"))
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.gist == nil {
			reg.Register(httpmock.REST("GET", "gists/1234"),
				httpmock.StatusStringResponse(404, "Not Found"))
		} else {
			reg.Register(httpmock.REST("GET", "gists/1234"),
				httpmock.JSONResponse(tt.gist))
			reg.Register(httpmock.GraphQL(`query UserCurrent\b`),
				httpmock.StringResponse(`{"data":{"viewer":{"login":"octocat"}}}`))
		}

		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}

		as, teardown := prompt.InitAskStubber()
		defer teardown()
		if tt.askStubs != nil {
			tt.askStubs(as)
		}

		if tt.opts == nil {
			tt.opts = &DeleteOptions{}
		}

		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		io, _, _, _ := iostreams.Test()
		io.SetStdoutTTY(!tt.nontty)
		io.SetStdinTTY(!tt.nontty)
		tt.opts.IO = io
		tt.opts.Selector = "1234"

		t.Run(tt.name, func(t *testing.T) {
			err := deleteRun(tt.opts)
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
