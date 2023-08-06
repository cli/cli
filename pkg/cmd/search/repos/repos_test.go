package repos

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdRepos(t *testing.T) {
	var trueBool = true
	tests := []struct {
		name    string
		input   string
		output  ReposOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no arguments",
			input:   "",
			wantErr: true,
			errMsg:  "specify search keywords or flags",
		},
		{
			name:  "keyword arguments",
			input: "some search terms",
			output: ReposOptions{
				Query: search.Query{Keywords: []string{"some", "search", "terms"}, Kind: "repositories", Limit: 30},
			},
		},
		{
			name:  "web flag",
			input: "--web",
			output: ReposOptions{
				Query:   search.Query{Keywords: []string{}, Kind: "repositories", Limit: 30},
				WebMode: true,
			},
		},
		{
			name:   "limit flag",
			input:  "--limit 10",
			output: ReposOptions{Query: search.Query{Keywords: []string{}, Kind: "repositories", Limit: 10}},
		},
		{
			name:    "invalid limit flag",
			input:   "--limit 1001",
			wantErr: true,
			errMsg:  "`--limit` must be between 1 and 1000",
		},
		{
			name:  "order flag",
			input: "--order asc",
			output: ReposOptions{
				Query: search.Query{Keywords: []string{}, Kind: "repositories", Limit: 30, Order: "asc"},
			},
		},
		{
			name:    "invalid order flag",
			input:   "--order invalid",
			wantErr: true,
			errMsg:  "invalid argument \"invalid\" for \"--order\" flag: valid values are {asc|desc}",
		},
		{
			name: "qualifier flags",
			input: `
      --archived
      --created=created
      --followers=1
      --include-forks=true
      --forks=2
      --good-first-issues=3
      --help-wanted-issues=4
      --match=description,readme
      --language=language
      --license=license
      --owner=owner
      --updated=updated
      --size=5
      --stars=6
      --topic=topic
      --number-topics=7
      --visibility=public
      `,
			output: ReposOptions{
				Query: search.Query{
					Keywords: []string{},
					Kind:     "repositories",
					Limit:    30,
					Qualifiers: search.Qualifiers{
						Archived:         &trueBool,
						Created:          "created",
						Followers:        "1",
						Fork:             "true",
						Forks:            "2",
						GoodFirstIssues:  "3",
						HelpWantedIssues: "4",
						In:               []string{"description", "readme"},
						Language:         "language",
						License:          []string{"license"},
						Pushed:           "updated",
						Size:             "5",
						Stars:            "6",
						Topic:            []string{"topic"},
						Topics:           "7",
						User:             []string{"owner"},
						Is:               []string{"public"},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *ReposOptions
			cmd := NewCmdRepos(f, func(opts *ReposOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.Query, gotOpts.Query)
			assert.Equal(t, tt.output.WebMode, gotOpts.WebMode)
		})
	}
}

func TestReposRun(t *testing.T) {
	var now = time.Date(2022, 2, 28, 12, 30, 0, 0, time.UTC)
	var updatedAt = time.Date(2021, 2, 28, 12, 30, 0, 0, time.UTC)
	var query = search.Query{
		Keywords: []string{"cli"},
		Kind:     "repositories",
		Limit:    30,
		Qualifiers: search.Qualifiers{
			Stars: ">50",
			Topic: []string{"golang"},
		},
	}
	tests := []struct {
		errMsg     string
		name       string
		opts       *ReposOptions
		tty        bool
		wantErr    bool
		wantStderr string
		wantStdout string
	}{
		{
			name: "displays results tty",
			opts: &ReposOptions{
				Query: query,
				Searcher: &search.SearcherMock{
					RepositoriesFunc: func(query search.Query) (search.RepositoriesResult, error) {
						return search.RepositoriesResult{
							IncompleteResults: false,
							Items: []search.Repository{
								{FullName: "test/cli", Description: "of course", IsPrivate: true, IsArchived: true, UpdatedAt: updatedAt, Visibility: "private"},
								{FullName: "test/cliing", Description: "wow", IsFork: true, UpdatedAt: updatedAt, Visibility: "public"},
								{FullName: "cli/cli", Description: "so much", IsArchived: false, UpdatedAt: updatedAt, Visibility: "internal"},
							},
							Total: 300,
						}, nil
					},
				},
			},
			tty:        true,
			wantStdout: "\nShowing 3 of 300 repositories\n\nNAME         DESCRIPTION  VISIBILITY         UPDATED\ntest/cli     of course    private, archived  about 1 year ago\ntest/cliing  wow          public, fork       about 1 year ago\ncli/cli      so much      internal           about 1 year ago\n",
		},
		{
			name: "displays results notty",
			opts: &ReposOptions{
				Query: query,
				Searcher: &search.SearcherMock{
					RepositoriesFunc: func(query search.Query) (search.RepositoriesResult, error) {
						return search.RepositoriesResult{
							IncompleteResults: false,
							Items: []search.Repository{
								{FullName: "test/cli", Description: "of course", IsPrivate: true, IsArchived: true, UpdatedAt: updatedAt, Visibility: "private"},
								{FullName: "test/cliing", Description: "wow", IsFork: true, UpdatedAt: updatedAt, Visibility: "public"},
								{FullName: "cli/cli", Description: "so much", IsArchived: false, UpdatedAt: updatedAt, Visibility: "internal"},
							},
							Total: 300,
						}, nil
					},
				},
			},
			wantStdout: "test/cli\tof course\tprivate, archived\t2021-02-28T12:30:00Z\ntest/cliing\twow\tpublic, fork\t2021-02-28T12:30:00Z\ncli/cli\tso much\tinternal\t2021-02-28T12:30:00Z\n",
		},
		{
			name: "displays no results",
			opts: &ReposOptions{
				Query: query,
				Searcher: &search.SearcherMock{
					RepositoriesFunc: func(query search.Query) (search.RepositoriesResult, error) {
						return search.RepositoriesResult{}, nil
					},
				},
			},
			wantErr: true,
			errMsg:  "no repositories matched your search",
		},
		{
			name: "displays search error",
			opts: &ReposOptions{
				Query: query,
				Searcher: &search.SearcherMock{
					RepositoriesFunc: func(query search.Query) (search.RepositoriesResult, error) {
						return search.RepositoriesResult{}, fmt.Errorf("error with query")
					},
				},
			},
			errMsg:  "error with query",
			wantErr: true,
		},
		{
			name: "opens browser for web mode tty",
			opts: &ReposOptions{
				Browser: &browser.Stub{},
				Query:   query,
				Searcher: &search.SearcherMock{
					URLFunc: func(query search.Query) string {
						return "https://github.com/search?type=repositories&q=cli"
					},
				},
				WebMode: true,
			},
			tty:        true,
			wantStderr: "Opening github.com/search in your browser.\n",
		},
		{
			name: "opens browser for web mode notty",
			opts: &ReposOptions{
				Browser: &browser.Stub{},
				Query:   query,
				Searcher: &search.SearcherMock{
					URLFunc: func(query search.Query) string {
						return "https://github.com/search?type=repositories&q=cli"
					},
				},
				WebMode: true,
			},
		},
	}
	for _, tt := range tests {
		ios, _, stdout, stderr := iostreams.Test()
		ios.SetStdinTTY(tt.tty)
		ios.SetStdoutTTY(tt.tty)
		ios.SetStderrTTY(tt.tty)
		tt.opts.IO = ios
		tt.opts.Now = now
		t.Run(tt.name, func(t *testing.T) {
			err := reposRun(tt.opts)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			} else if err != nil {
				t.Fatalf("reposRun unexpected error: %v", err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
