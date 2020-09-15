package list

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/pkg/cmd/gist/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants ListOptions
	}{
		{
			name: "no arguments",
			cli:  "",
			wants: ListOptions{
				Limit:      10,
				Visibility: "all",
			},
		},
		{
			name: "public",
			cli:  "--public",
			wants: ListOptions{
				Limit:      10,
				Visibility: "public",
			},
		},
		{
			name: "secret",
			cli:  "--secret",
			wants: ListOptions{
				Limit:      10,
				Visibility: "secret",
			},
		},
		{
			name: "public and secret",
			cli:  "--secret --public",
			wants: ListOptions{
				Limit:      10,
				Visibility: "all",
			},
		},
		{
			name: "limit",
			cli:  "--limit 30",
			wants: ListOptions{
				Limit:      30,
				Visibility: "all",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *ListOptions
			cmd := NewCmdList(f, func(opts *ListOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Visibility, gotOpts.Visibility)
			assert.Equal(t, tt.wants.Limit, gotOpts.Limit)
		})
	}
}

func Test_listRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *ListOptions
		wantOut string
		stubs   func(*httpmock.Registry)
		nontty  bool
	}{
		{
			name: "no gists",
			opts: &ListOptions{},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "gists"),
					httpmock.JSONResponse([]shared.Gist{}))

			},
			wantOut: "",
		},
		{
			name:    "default behavior",
			opts:    &ListOptions{},
			wantOut: "1234567890                        1 file    public  about 6 hours ago\n2345678901  tea leaves thwart...  2 files   secret  about 6 hours ago\n3456789012  short desc            11 files  secret  about 6 hours ago\n",
		},
		{
			name:    "with public filter",
			opts:    &ListOptions{Visibility: "public"},
			wantOut: "1234567890    1 file  public  about 6 hours ago\n",
		},
		{
			name:    "with secret filter",
			opts:    &ListOptions{Visibility: "secret"},
			wantOut: "2345678901  tea leaves thwart...  2 files   secret  about 6 hours ago\n3456789012  short desc            11 files  secret  about 6 hours ago\n",
		},
		{
			name:    "with limit",
			opts:    &ListOptions{Limit: 1},
			wantOut: "1234567890    1 file  public  about 6 hours ago\n",
		},
		{
			name:    "nontty output",
			opts:    &ListOptions{},
			wantOut: "",
			nontty:  true,
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.stubs == nil {
			reg.Register(httpmock.REST("GET", "gists"),
				httpmock.JSONResponse([]shared.Gist{
					{
						ID:          "1234567890",
						Description: "",
						Files: map[string]*shared.GistFile{
							"cool.txt": {
								Content: "lol",
							},
						},
						Public: true,
					},
					{
						ID:          "2345678901",
						Description: "tea leaves thwart those who court catastrophe",
						Files: map[string]*shared.GistFile{
							"gistfile0.txt": {
								Content: "lolol",
							},
							"gistfile1.txt": {
								Content: "lololol",
							},
						},
						Public: false,
					},
					{
						ID:          "3456789012",
						Description: "short desc",
						Files: map[string]*shared.GistFile{
							"gistfile0.txt":  {},
							"gistfile1.txt":  {},
							"gistfile2.txt":  {},
							"gistfile3.txt":  {},
							"gistfile4.txt":  {},
							"gistfile5.txt":  {},
							"gistfile6.txt":  {},
							"gistfile7.txt":  {},
							"gistfile8.txt":  {},
							"gistfile9.txt":  {},
							"gistfile10.txt": {},
						},
						Public: false,
					},
				}))
		} else {
			tt.stubs(reg)
		}

		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		tt.opts.Since = func(t time.Time) time.Duration {
			d, _ := time.ParseDuration("6h")
			return d
		}

		io, _, stdout, _ := iostreams.Test()
		io.SetStdoutTTY(!tt.nontty)
		tt.opts.IO = io

		if tt.opts.Limit == 0 {
			tt.opts.Limit = 10
		}

		if tt.opts.Visibility == "" {
			tt.opts.Visibility = "all"
		}
		t.Run(tt.name, func(t *testing.T) {
			err := listRun(tt.opts)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)
		})
	}
}
