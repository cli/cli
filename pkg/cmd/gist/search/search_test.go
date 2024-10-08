package search

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

func TestNewCmdSearch(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants SearchOptions
	}{
		{
			name: "pattern only",
			cli:  "octo",
			wants: SearchOptions{
				Pattern:    regexp.MustCompilePOSIX("octo"),
				Limit:      10,
				Visibility: "all",
			},
		},
		{
			name: "public",
			cli:  "--public octo",
			wants: SearchOptions{
				Pattern:    regexp.MustCompilePOSIX("octo"),
				Limit:      10,
				Visibility: "public",
			},
		},
		{
			name: "secret",
			cli:  `"foo|bar" --secret`,
			wants: SearchOptions{
				Pattern:    regexp.MustCompilePOSIX("foo|bar"),
				Limit:      10,
				Visibility: "secret",
			},
		},
		{
			name: "secret with explicit false value",
			cli:  "--secret=false octo",
			wants: SearchOptions{
				Pattern:    regexp.MustCompilePOSIX("octo"),
				Limit:      10,
				Visibility: "all",
			},
		},
		{
			name: "public and secret",
			cli:  "--secret --public octo",
			wants: SearchOptions{
				Pattern:    regexp.MustCompilePOSIX("octo"),
				Limit:      10,
				Visibility: "secret",
			},
		},
		{
			name: "limit",
			cli:  "--limit 30 octo",
			wants: SearchOptions{
				Pattern:    regexp.MustCompilePOSIX("octo"),
				Limit:      30,
				Visibility: "all",
			},
		},
		{
			name: "all arguments",
			cli:  "--public --secret --filename --code --limit 30 octo",
			wants: SearchOptions{
				Pattern:    regexp.MustCompilePOSIX("octo"),
				Filename:   true,
				Code:       true,
				Limit:      30,
				Visibility: "secret",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *SearchOptions
			cmd := NewCmdSearch(f, func(opts *SearchOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Pattern.String(), gotOpts.Pattern.String())
			assert.Equal(t, tt.wants.Filename, gotOpts.Filename)
			assert.Equal(t, tt.wants.Code, gotOpts.Code)
			assert.Equal(t, tt.wants.Visibility, gotOpts.Visibility)
			assert.Equal(t, tt.wants.Limit, gotOpts.Limit)
		})
	}
}

func Test_searchRun(t *testing.T) {
	const query = `query GistList\b`
	sixHours, _ := time.ParseDuration("6h")
	sixHoursAgo := time.Now().Add(-sixHours)

	tests := []struct {
		name    string
		opts    *SearchOptions
		wantErr bool
		wantOut string
		stubs   func(*httpmock.Registry)
		nontty  bool
	}{
		{
			name: "no gists",
			opts: &SearchOptions{},
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
			name: "description only",
			opts: &SearchOptions{
				Pattern: regexp.MustCompilePOSIX("foo"),
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234567890",
								"files": [{ "name": "cool.txt" }],
								"description": "foo example",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "4567890123",
								"files": [{ "name": "gistfile0.txt" }],
								"description": "bar example",
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
				1234567890  foo example  1 file  public      about 6 hours ago
			`),
		},
		{
			name: "no results with public filter",
			opts: &SearchOptions{
				Pattern:    regexp.MustCompilePOSIX("foo"),
				Visibility: "public",
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "4567890123",
								"files": [{ "name": "gistfile0.txt" }],
								"description": "bar example",
								"updatedAt": "%[1]v",
								"isPublic": true
							}
						] } } } }`,
						sixHoursAgo.Format(time.RFC3339),
					)),
				)
			},
			wantErr: true,
		},
		{
			name: "description only",
			opts: &SearchOptions{
				Pattern: regexp.MustCompilePOSIX("foo"),
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234567890",
								"files": [{ "name": "cool.txt" }],
								"description": "foo example",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "4567890123",
								"files": [{ "name": "gistfile0.txt" }],
								"description": "bar example",
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
				1234567890  foo example  1 file  public      about 6 hours ago
			`),
		},
		{
			name: "include filenames",
			opts: &SearchOptions{
				Pattern:  regexp.MustCompilePOSIX("gistfile7"),
				Filename: true,
			},
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
								"description": "foo example",
								"updatedAt": "%[1]v",
								"isPublic": true
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
								"description": "bar example",
								"updatedAt": "%[1]v",
								"isPublic": false
							}
						] } } } }`,
						sixHoursAgo.Format(time.RFC3339),
					)),
				)
			},
			wantOut: heredoc.Doc(`
				ID          DESCRIPTION  FILES     VISIBILITY  UPDATED
				3456789012  bar example  11 files  secret      about 6 hours ago
			`),
		},
		{
			name: "include code",
			opts: &SearchOptions{
				Pattern: regexp.MustCompilePOSIX("octo"),
				Code:    true,
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(query),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
								{
									"name": "2345678901",
									"files": [
										{
											"name": "gistfile0.txt",
											"text": "octo"
										}
									],
									"description": "foo example",
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
					2345678901  foo example  1 file  public      about 6 hours ago
				`),
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
		ios.SetStdoutTTY(!tt.nontty)
		tt.opts.IO = ios

		if tt.opts.Limit == 0 {
			tt.opts.Limit = 10
		}

		if tt.opts.Visibility == "" {
			tt.opts.Visibility = "all"
		}
		t.Run(tt.name, func(t *testing.T) {
			err := searchRun(tt.opts)
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
