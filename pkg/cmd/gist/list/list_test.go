package list

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    ListOptions
		wantsErr bool
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
		{
			name:     "invalid limit",
			cli:      "--limit 0",
			wantsErr: true,
		},
		{
			name: "filter and include-content",
			cli:  "--filter octo --include-content",
			wants: ListOptions{
				Limit:          10,
				Filter:         regexp.MustCompilePOSIX("octo"),
				IncludeContent: true,
				Visibility:     "all",
			},
		},
		{
			name:     "invalid filter",
			cli:      "--filter octo(",
			wantsErr: true,
		},
		{
			name:     "include content without filter",
			cli:      "--include-content",
			wantsErr: true,
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
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
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
		wantErr bool
		wantOut string
		stubs   func(*httpmock.Registry)
		color   bool
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
			wantErr: true,
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
				ID          DESCRIPTION                  FILES     VISIBILITY  UPDATED
				1234567890  cool.txt                     1 file    public      about 6 hours ago
				4567890123                               1 file    public      about 6 hours ago
				2345678901  tea leaves thwart those ...  2 files   secret      about 6 hours ago
				3456789012  short desc                   11 files  secret      about 6 hours ago
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
				ID          DESCRIPTION  FILES   VISIBILITY  UPDATED
				1234567890  cool.txt     1 file  public      about 6 hours ago
				4567890123               1 file  public      about 6 hours ago
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
				ID          DESCRIPTION                  FILES     VISIBILITY  UPDATED
				2345678901  tea leaves thwart those ...  2 files   secret      about 6 hours ago
				3456789012  short desc                   11 files  secret      about 6 hours ago
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
			wantOut: heredoc.Doc(`
				ID          DESCRIPTION  FILES   VISIBILITY  UPDATED
				1234567890  cool.txt     1 file  public      about 6 hours ago
			`),
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
		{
			name: "filtered",
			opts: &ListOptions{
				Filter:     regexp.MustCompile("octo"),
				Visibility: "all",
			},
			nontty: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234",
								"files": [
									{ "name": "main.txt", "text": "foo" }
								],
								"description": "octo match in the description",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "2345",
								"files": [
									{ "name": "main.txt", "text": "foo" },
									{ "name": "octo.txt", "text": "bar" }
								],
								"description": "match in the file name",
								"updatedAt": "%[1]v",
								"isPublic": false
							},
							{
								"name": "3456",
								"files": [
									{ "name": "main.txt", "text": "octo in the text" }
								],
								"description": "match in the file text",
								"updatedAt": "%[1]v",
								"isPublic": true
							}
						] } } } }`,
						absTime.Format(time.RFC3339),
					)),
				)
			},
			wantOut: heredoc.Docf(`
				1234%[1]socto match in the description%[1]s1 file%[1]spublic%[1]s2020-07-30T15:24:28Z
				2345%[1]smatch in the file name%[1]s2 files%[1]ssecret%[1]s2020-07-30T15:24:28Z
			`, "\t"),
		},
		{
			name: "filtered (tty)",
			opts: &ListOptions{
				Filter:     regexp.MustCompile("octo"),
				Visibility: "all",
			},
			color: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234",
								"files": [
									{ "name": "main.txt", "text": "foo" }
								],
								"description": "octo match in the description",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "2345",
								"files": [
									{ "name": "main.txt", "text": "foo" },
									{ "name": "octo.txt", "text": "bar" }
								],
								"description": "match in the file name",
								"updatedAt": "%[1]v",
								"isPublic": false
							},
							{
								"name": "3456",
								"files": [
									{ "name": "main.txt", "text": "octo in the text" }
								],
								"description": "match in the file text",
								"updatedAt": "%[1]v",
								"isPublic": true
							}
						] } } } }`,
						sixHoursAgo.Format(time.RFC3339),
					)),
				)
			},
			wantOut: heredoc.Docf(`
				%[1]s[0;2;4;37mID  %[1]s[0m  %[1]s[0;2;4;37mDESCRIPTION                  %[1]s[0m  %[1]s[0;2;4;37mFILES  %[1]s[0m  %[1]s[0;2;4;37mVISIBILITY%[1]s[0m  %[1]s[0;2;4;37mUPDATED          %[1]s[0m
				1234  %[1]s[0;30;43mocto%[1]s[0m%[1]s[0;1;39m match in the description%[1]s[0m  1 file   %[1]s[0;32mpublic    %[1]s[0m  %[1]s[38;5;242mabout 6 hours ago%[1]s[m
				2345  %[1]s[0;1;39mmatch in the file name       %[1]s[0m  %[1]s[0;30;43m2 files%[1]s[0m  %[1]s[0;31msecret    %[1]s[0m  %[1]s[38;5;242mabout 6 hours ago%[1]s[m
			`, "\x1b"),
		},
		{
			name: "filtered with content",
			opts: &ListOptions{
				Filter:         regexp.MustCompile("octo"),
				IncludeContent: true,
				Visibility:     "all",
			},
			nontty: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234",
								"files": [
									{ "name": "main.txt", "text": "foo" }
								],
								"description": "octo match in the description",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "2345",
								"files": [
									{ "name": "main.txt", "text": "foo" },
									{ "name": "octo.txt", "text": "bar" }
								],
								"description": "match in the file name",
								"updatedAt": "%[1]v",
								"isPublic": false
							},
							{
								"name": "3456",
								"files": [
									{ "name": "main.txt", "text": "octo in the text" }
								],
								"description": "match in the file text",
								"updatedAt": "%[1]v",
								"isPublic": true
							}
						] } } } }`,
						absTime.Format(time.RFC3339),
					)),
				)
			},
			wantOut: heredoc.Doc(`
				1234 main.txt
				    octo match in the description
				
				2345 octo.txt
				    match in the file name

				3456 main.txt
				    match in the file text
				        octo in the text
				
			`),
		},
		{
			name: "filtered with content (tty)",
			opts: &ListOptions{
				Filter:         regexp.MustCompile("octo"),
				IncludeContent: true,
				Visibility:     "all",
			},
			color: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234",
								"files": [
									{ "name": "main.txt", "text": "foo" }
								],
								"description": "octo match in the description",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "2345",
								"files": [
									{ "name": "main.txt", "text": "foo" },
									{ "name": "octo.txt", "text": "bar" }
								],
								"description": "match in the file name",
								"updatedAt": "%[1]v",
								"isPublic": false
							},
							{
								"name": "3456",
								"files": [
									{ "name": "main.txt", "text": "octo in the text" }
								],
								"description": "match in the file text",
								"updatedAt": "%[1]v",
								"isPublic": true
							}
						] } } } }`,
						sixHoursAgo.Format(time.RFC3339),
					)),
				)
			},
			wantOut: heredoc.Docf(`
				%[1]s[0;34m1234%[1]s[0m %[1]s[0;32mmain.txt%[1]s[0m
				    %[1]s[0;30;43mocto%[1]s[0m%[1]s[0;1;39m match in the description%[1]s[0m
				
				%[1]s[0;34m2345%[1]s[0m %[1]s[0;30;43mocto%[1]s[0m%[1]s[0;32m.txt%[1]s[0m
				    %[1]s[0;1;39mmatch in the file name%[1]s[0m
				
				%[1]s[0;34m3456%[1]s[0m %[1]s[0;32mmain.txt%[1]s[0m
				    %[1]s[0;1;39mmatch in the file text%[1]s[0m
				        %[1]s[0;30;43mocto%[1]s[0m in the text
				
			`, "\x1b"),
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		tt.stubs(reg)

		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		tt.opts.Config = func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		}

		ios, _, stdout, _ := iostreams.Test()
		ios.SetColorEnabled(tt.color)
		ios.SetStdoutTTY(!tt.nontty)
		tt.opts.IO = ios

		if tt.opts.Limit == 0 {
			tt.opts.Limit = 10
		}

		if tt.opts.Visibility == "" {
			tt.opts.Visibility = "all"
		}
		t.Run(tt.name, func(t *testing.T) {
			err := listRun(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)
		})
	}
}

func Test_highlightMatch(t *testing.T) {
	regex := regexp.MustCompilePOSIX(`[Oo]cto`)
	tests := []struct {
		name  string
		input string
		color bool
		want  string
	}{
		{
			name:  "single match",
			input: "Octo",
			want:  "Octo",
		},
		{
			name:  "single match (color)",
			input: "Octo",
			color: true,
			want:  "\x1b[0;30;43mOcto\x1b[0m",
		},
		{
			name:  "single match with extra",
			input: "Hello, Octocat!",
			want:  "Hello, Octocat!",
		},
		{
			name:  "single match with extra (color)",
			input: "Hello, Octocat!",
			color: true,
			want:  "\x1b[0;34mHello, \x1b[0m\x1b[0;30;43mOcto\x1b[0m\x1b[0;34mcat!\x1b[0m",
		},
		{
			name:  "multiple matches",
			input: "Octocat/octo",
			want:  "Octocat/octo",
		},
		{
			name:  "multiple matches (color)",
			input: "Octocat/octo",
			color: true,
			want:  "\x1b[0;30;43mOcto\x1b[0m\x1b[0;34mcat/\x1b[0m\x1b[0;30;43mocto\x1b[0m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := iostreams.NewColorScheme(tt.color, false, false)

			matched := false
			got, err := highlightMatch(tt.input, regex, &matched, cs.Blue, cs.Highlight)
			assert.NoError(t, err)
			assert.True(t, matched)
			assert.Equal(t, tt.want, got)
		})
	}
}
