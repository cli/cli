package list

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/config"
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
			name: "secret with explicit false value",
			cli:  "--secret=false",
			wants: ListOptions{
				Limit:      10,
				Visibility: "all",
			},
		},
		{
			name: "public and secret",
			cli:  "--secret --public",
			wants: ListOptions{
				Limit:      10,
				Visibility: "secret",
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
	const query = `query GistList\b`
	sixHours, _ := time.ParseDuration("6h")
	sixHoursAgo := time.Now().Add(-sixHours)
	absTime, _ := time.Parse(time.RFC3339, "2020-07-30T15:24:28Z")

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
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(`{ "data": { "viewer": {
						"gists": { "nodes": [] }
					} } }`))
			},
			wantOut: "",
		},
		{
			name: "default behavior",
			opts: &ListOptions{},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234567890",
								"files": [{ "name": "cool.txt" }],
								"description": "",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "4567890123",
								"files": [{ "name": "gistfile0.txt" }],
								"description": "",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "2345678901",
								"files": [
									{ "name": "gistfile0.txt" },
									{ "name": "gistfile1.txt" }
								],
								"description": "tea leaves thwart those who court catastrophe",
								"updatedAt": "%[1]v",
								"isPublic": false
							},
							{
								"name": "3456789012",
								"files": [
									{ "name": "gistfile0.txt" },
									{ "name": "gistfile1.txt" },
									{ "name": "gistfile2.txt" },
									{ "name": "gistfile3.txt" },
									{ "name": "gistfile4.txt" },
									{ "name": "gistfile5.txt" },
									{ "name": "gistfile6.txt" },
									{ "name": "gistfile7.txt" },
									{ "name": "gistfile9.txt" },
									{ "name": "gistfile10.txt" },
									{ "name": "gistfile11.txt" }
								],
								"description": "short desc",
								"updatedAt": "%[1]v",
								"isPublic": false
							}
						] } } } }`,
						sixHoursAgo.Format(time.RFC3339),
					)),
				)
			},
			wantOut: heredoc.Doc(`
				1234567890  cool.txt                         1 file    public  about 6 hours ago
				4567890123                                   1 file    public  about 6 hours ago
				2345678901  tea leaves thwart those who ...  2 files   secret  about 6 hours ago
				3456789012  short desc                       11 files  secret  about 6 hours ago
			`),
		},
		{
			name: "with public filter",
			opts: &ListOptions{Visibility: "public"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234567890",
								"files": [{ "name": "cool.txt" }],
								"description": "",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "4567890123",
								"files": [{ "name": "gistfile0.txt" }],
								"description": "",
								"updatedAt": "%[1]v",
								"isPublic": true
							}
						] } } } }`,
						sixHoursAgo.Format(time.RFC3339),
					)),
				)
			},
			wantOut: heredoc.Doc(`
				1234567890  cool.txt  1 file  public  about 6 hours ago
				4567890123            1 file  public  about 6 hours ago
			`),
		},
		{
			name: "with secret filter",
			opts: &ListOptions{Visibility: "secret"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "2345678901",
								"files": [
									{ "name": "gistfile0.txt" },
									{ "name": "gistfile1.txt" }
								],
								"description": "tea leaves thwart those who court catastrophe",
								"updatedAt": "%[1]v",
								"isPublic": false
							},
							{
								"name": "3456789012",
								"files": [
									{ "name": "gistfile0.txt" },
									{ "name": "gistfile1.txt" },
									{ "name": "gistfile2.txt" },
									{ "name": "gistfile3.txt" },
									{ "name": "gistfile4.txt" },
									{ "name": "gistfile5.txt" },
									{ "name": "gistfile6.txt" },
									{ "name": "gistfile7.txt" },
									{ "name": "gistfile9.txt" },
									{ "name": "gistfile10.txt" },
									{ "name": "gistfile11.txt" }
								],
								"description": "short desc",
								"updatedAt": "%[1]v",
								"isPublic": false
							}
						] } } } }`,
						sixHoursAgo.Format(time.RFC3339),
					)),
				)
			},
			wantOut: heredoc.Doc(`
				2345678901  tea leaves thwart those who ...  2 files   secret  about 6 hours ago
				3456789012  short desc                       11 files  secret  about 6 hours ago
			`),
		},
		{
			name: "with limit",
			opts: &ListOptions{Limit: 1},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234567890",
								"files": [{ "name": "cool.txt" }],
								"description": "",
								"updatedAt": "%v",
								"isPublic": true
							}
						] } } } }`,
						sixHoursAgo.Format(time.RFC3339),
					)),
				)
			},
			wantOut: "1234567890  cool.txt  1 file  public  about 6 hours ago\n",
		},
		{
			name: "nontty output",
			opts: &ListOptions{},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234567890",
								"files": [{ "name": "cool.txt" }],
								"description": "",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "4567890123",
								"files": [{ "name": "gistfile0.txt" }],
								"description": "",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "2345678901",
								"files": [
									{ "name": "gistfile0.txt" },
									{ "name": "gistfile1.txt" }
								],
								"description": "tea leaves thwart those who court catastrophe",
								"updatedAt": "%[1]v",
								"isPublic": false
							},
							{
								"name": "3456789012",
								"files": [
									{ "name": "gistfile0.txt" },
									{ "name": "gistfile1.txt" },
									{ "name": "gistfile2.txt" },
									{ "name": "gistfile3.txt" },
									{ "name": "gistfile4.txt" },
									{ "name": "gistfile5.txt" },
									{ "name": "gistfile6.txt" },
									{ "name": "gistfile7.txt" },
									{ "name": "gistfile9.txt" },
									{ "name": "gistfile10.txt" },
									{ "name": "gistfile11.txt" }
								],
								"description": "short desc",
								"updatedAt": "%[1]v",
								"isPublic": false
							}
						] } } } }`,
						absTime.Format(time.RFC3339),
					)),
				)
			},
			wantOut: heredoc.Doc(`
				1234567890	cool.txt	1 file	public	2020-07-30T15:24:28Z
				4567890123		1 file	public	2020-07-30T15:24:28Z
				2345678901	tea leaves thwart those who court catastrophe	2 files	secret	2020-07-30T15:24:28Z
				3456789012	short desc	11 files	secret	2020-07-30T15:24:28Z
			`),
			nontty: true,
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		tt.stubs(reg)

		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		tt.opts.Config = func() (config.Config, error) {
			return config.NewBlankConfig(), nil
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
