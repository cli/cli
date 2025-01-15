package commits

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

func TestNewCmdCommits(t *testing.T) {
	var trueBool = true
	tests := []struct {
		name    string
		input   string
		output  CommitsOptions
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
			output: CommitsOptions{
				Query: search.Query{Keywords: []string{"some", "search", "terms"}, Kind: "commits", Limit: 30},
			},
		},
		{
			name:  "web flag",
			input: "--web",
			output: CommitsOptions{
				Query:   search.Query{Keywords: []string{}, Kind: "commits", Limit: 30},
				WebMode: true,
			},
		},
		{
			name:   "limit flag",
			input:  "--limit 10",
			output: CommitsOptions{Query: search.Query{Keywords: []string{}, Kind: "commits", Limit: 10}},
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
			output: CommitsOptions{
				Query: search.Query{Keywords: []string{}, Kind: "commits", Limit: 30, Order: "asc"},
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
		    --author=foo
		    --author-date=01-01-2000
		    --author-email=foo@example.com
		    --author-name=Foo
		    --committer=bar
		    --committer-date=01-02-2000
		    --committer-email=bar@example.com
		    --committer-name=Bar
		    --hash=aaa
		    --merge
		    --parent=bbb
		    --repo=owner/repo
		    --tree=ccc
		    --owner=owner
		    --visibility=public
		    `,
			output: CommitsOptions{
				Query: search.Query{
					Keywords: []string{},
					Kind:     "commits",
					Limit:    30,
					Qualifiers: search.Qualifiers{
						Author:         "foo",
						AuthorDate:     "01-01-2000",
						AuthorEmail:    "foo@example.com",
						AuthorName:     "Foo",
						Committer:      "bar",
						CommitterDate:  "01-02-2000",
						CommitterEmail: "bar@example.com",
						CommitterName:  "Bar",
						Hash:           "aaa",
						Merge:          &trueBool,
						Parent:         "bbb",
						Repo:           []string{"owner/repo"},
						Tree:           "ccc",
						User:           []string{"owner"},
						Is:             []string{"public"},
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
			var gotOpts *CommitsOptions
			cmd := NewCmdCommits(f, func(opts *CommitsOptions) error {
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

func TestCommitsRun(t *testing.T) {
	var now = time.Date(2023, 1, 17, 12, 30, 0, 0, time.UTC)
	var author = search.CommitUser{Date: time.Date(2022, 12, 27, 11, 30, 0, 0, time.UTC)}
	var committer = search.CommitUser{Date: time.Date(2022, 12, 28, 12, 30, 0, 0, time.UTC)}
	var query = search.Query{
		Keywords:   []string{"cli"},
		Kind:       "commits",
		Limit:      30,
		Qualifiers: search.Qualifiers{},
	}
	tests := []struct {
		errMsg     string
		name       string
		opts       *CommitsOptions
		tty        bool
		wantErr    bool
		wantStderr string
		wantStdout string
	}{
		{
			name: "displays results tty",
			opts: &CommitsOptions{
				Query: query,
				Searcher: &search.SearcherMock{
					CommitsFunc: func(query search.Query) (search.CommitsResult, error) {
						return search.CommitsResult{
							IncompleteResults: false,
							Items: []search.Commit{
								{
									Author: search.User{Login: "monalisa"},
									Info:   search.CommitInfo{Author: author, Committer: committer, Message: "hello"},
									Repo:   search.Repository{FullName: "test/cli"},
									Sha:    "aaaaaaaa",
								},
								{
									Author: search.User{Login: "johnnytest"},
									Info:   search.CommitInfo{Author: author, Committer: committer, Message: "hi"},
									Repo:   search.Repository{FullName: "test/cliing", IsPrivate: true},
									Sha:    "bbbbbbbb",
								},
								{
									Author: search.User{Login: "hubot"},
									Info:   search.CommitInfo{Author: author, Committer: committer, Message: "greetings"},
									Repo:   search.Repository{FullName: "cli/cli"},
									Sha:    "cccccccc",
								},
							},
							Total: 300,
						}, nil
					},
				},
			},
			tty:        true,
			wantStdout: "\nShowing 3 of 300 commits\n\nREPO         SHA       MESSAGE    AUTHOR      CREATED\ntest/cli     aaaaaaaa  hello      monalisa    about 21 days ago\ntest/cliing  bbbbbbbb  hi         johnnytest  about 21 days ago\ncli/cli      cccccccc  greetings  hubot       about 21 days ago\n",
		},
		{
			name: "displays results notty",
			opts: &CommitsOptions{
				Query: query,
				Searcher: &search.SearcherMock{
					CommitsFunc: func(query search.Query) (search.CommitsResult, error) {
						return search.CommitsResult{
							IncompleteResults: false,
							Items: []search.Commit{
								{
									Author: search.User{Login: "monalisa"},
									Info:   search.CommitInfo{Author: author, Committer: committer, Message: "hello"},
									Repo:   search.Repository{FullName: "test/cli"},
									Sha:    "aaaaaaaa",
								},
								{
									Author: search.User{Login: "johnnytest"},
									Info:   search.CommitInfo{Author: author, Committer: committer, Message: "hi"},
									Repo:   search.Repository{FullName: "test/cliing", IsPrivate: true},
									Sha:    "bbbbbbbb",
								},
								{
									Author: search.User{Login: "hubot"},
									Info:   search.CommitInfo{Author: author, Committer: committer, Message: "greetings"},
									Repo:   search.Repository{FullName: "cli/cli"},
									Sha:    "cccccccc",
								},
							},
							Total: 300,
						}, nil
					},
				},
			},
			wantStdout: "test/cli\taaaaaaaa\thello\tmonalisa\t2022-12-27T11:30:00Z\ntest/cliing\tbbbbbbbb\thi\tjohnnytest\t2022-12-27T11:30:00Z\ncli/cli\tcccccccc\tgreetings\thubot\t2022-12-27T11:30:00Z\n",
		},
		{
			name: "displays no results",
			opts: &CommitsOptions{
				Query: query,
				Searcher: &search.SearcherMock{
					CommitsFunc: func(query search.Query) (search.CommitsResult, error) {
						return search.CommitsResult{}, nil
					},
				},
			},
			wantErr: true,
			errMsg:  "no commits matched your search",
		},
		{
			name: "displays search error",
			opts: &CommitsOptions{
				Query: query,
				Searcher: &search.SearcherMock{
					CommitsFunc: func(query search.Query) (search.CommitsResult, error) {
						return search.CommitsResult{}, fmt.Errorf("error with query")
					},
				},
			},
			errMsg:  "error with query",
			wantErr: true,
		},
		{
			name: "opens browser for web mode tty",
			opts: &CommitsOptions{
				Browser: &browser.Stub{},
				Query:   query,
				Searcher: &search.SearcherMock{
					URLFunc: func(query search.Query) string {
						return "https://github.com/search?type=commits&q=cli"
					},
				},
				WebMode: true,
			},
			tty:        true,
			wantStderr: "Opening https://github.com/search in your browser.\n",
		},
		{
			name: "opens browser for web mode notty",
			opts: &CommitsOptions{
				Browser: &browser.Stub{},
				Query:   query,
				Searcher: &search.SearcherMock{
					URLFunc: func(query search.Query) string {
						return "https://github.com/search?type=commits&q=cli"
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
			err := commitsRun(tt.opts)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			} else if err != nil {
				t.Fatalf("commitsRun unexpected error: %v", err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
