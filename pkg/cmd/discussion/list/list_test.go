package list

import (
	"bytes"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/discussion/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/jsonfieldstest"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListJSONFields(t *testing.T) {
	jsonfieldstest.ExpectCommandToSupportJSONFields(t, NewCmdList, []string{
		"id",
		"number",
		"title",
		"body",
		"url",
		"closed",
		"stateReason",
		"author",
		"category",
		"labels",
		"answered",
		"answerChosenAt",
		"answerChosenBy",
		"createdAt",
		"updatedAt",
		"closedAt",
		"locked",
	})
}

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantsErr bool
		wantOpts ListOptions
	}{
		{
			name: "no flags",
			args: "",
			wantOpts: ListOptions{
				State: "open",
				Limit: 30,
				Sort:  "updated",
				Order: "desc",
			},
		},
		{
			name: "state flag",
			args: "--state closed",
			wantOpts: ListOptions{
				State: "closed",
				Limit: 30,
				Sort:  "updated",
				Order: "desc",
			},
		},
		{
			name:     "invalid state",
			args:     "--state invalid",
			wantsErr: true,
		},
		{
			name: "label flag",
			args: "--label bug,docs",
			wantOpts: ListOptions{
				Labels: []string{"bug", "docs"},
				State:  "open",
				Limit:  30,
				Sort:   "updated",
				Order:  "desc",
			},
		},
		{
			name: "author flag",
			args: "--author monalisa",
			wantOpts: ListOptions{
				Author: "monalisa",
				State:  "open",
				Limit:  30,
				Sort:   "updated",
				Order:  "desc",
			},
		},
		{
			name: "category flag",
			args: "--category general",
			wantOpts: ListOptions{
				Category: "general",
				State:    "open",
				Limit:    30,
				Sort:     "updated",
				Order:    "desc",
			},
		},
		{
			name: "limit flag",
			args: "--limit 10",
			wantOpts: ListOptions{
				State: "open",
				Limit: 10,
				Sort:  "updated",
				Order: "desc",
			},
		},
		{
			name:     "invalid limit",
			args:     "--limit 0",
			wantsErr: true,
		},
		{
			name: "web flag",
			args: "--web",
			wantOpts: ListOptions{
				WebMode: true,
				State:   "open",
				Limit:   30,
				Sort:    "updated",
				Order:   "desc",
			},
		},
		{
			name: "sort flag",
			args: "--sort created",
			wantOpts: ListOptions{
				State: "open",
				Limit: 30,
				Sort:  "created",
				Order: "desc",
			},
		},
		{
			name:     "invalid sort",
			args:     "--sort invalid",
			wantsErr: true,
		},
		{
			name: "order flag",
			args: "--order asc",
			wantOpts: ListOptions{
				State: "open",
				Limit: 30,
				Sort:  "updated",
				Order: "asc",
			},
		},
		{
			name:     "invalid order",
			args:     "--order invalid",
			wantsErr: true,
		},
		{
			name: "search flag",
			args: `--search "some query"`,
			wantOpts: ListOptions{
				Search: "some query",
				State:  "open",
				Limit:  30,
				Sort:   "updated",
				Order:  "desc",
			},
		},
		{
			name: "after flag",
			args: "--after CURSOR123",
			wantOpts: ListOptions{
				After: "CURSOR123",
				State: "open",
				Limit: 30,
				Sort:  "updated",
				Order: "desc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				Browser:   &browser.Stub{},
				BaseRepo:  func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil },
			}

			var gotOpts *ListOptions
			cmd := NewCmdList(f, func(o *ListOptions) error {
				gotOpts = o
				return nil
			})

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()

			if tt.wantsErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, gotOpts)

			assert.Equal(t, tt.wantOpts.State, gotOpts.State)
			assert.Equal(t, tt.wantOpts.Limit, gotOpts.Limit)
			assert.Equal(t, tt.wantOpts.Sort, gotOpts.Sort)
			assert.Equal(t, tt.wantOpts.Order, gotOpts.Order)
			assert.Equal(t, tt.wantOpts.Author, gotOpts.Author)
			assert.Equal(t, tt.wantOpts.Category, gotOpts.Category)
			assert.Equal(t, tt.wantOpts.Labels, gotOpts.Labels)
			assert.Equal(t, tt.wantOpts.Search, gotOpts.Search)
			assert.Equal(t, tt.wantOpts.After, gotOpts.After)
			assert.Equal(t, tt.wantOpts.WebMode, gotOpts.WebMode)
		})
	}
}

func TestListRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       ListOptions
		tty        bool
		clientStub func(*testing.T, *client.DiscussionClientMock)
		wantErr    string
		wantErrAs  any
		wantStdout string
		wantStderr string
		wantBrowse string
	}{
		{
			name: "tty output",
			tty:  true,
			opts: ListOptions{
				State: stateOpen,
				Limit: 30,
				Sort:  sortUpdated,
				Order: orderDesc,
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.ListFunc = func(repo ghrepo.Interface, filters client.ListFilters, after string, limit int) (*client.DiscussionListResult, error) {
					return sampleResult(), nil
				}
			},
			wantStdout: heredoc.Doc(`

				Showing 2 of 2 open discussions in OWNER/REPO

				ID   TITLE                  CATEGORY  LABELS  ANSWERED  UPDATED
				#42  Bug report discussion  General   bug     ✓         about 12 hours ago
				#41  Feature request        Ideas                       about 8 days ago
			`),
		},
		{
			name: "non-tty output",
			tty:  false,
			opts: ListOptions{
				State: stateOpen,
				Limit: 30,
				Sort:  sortUpdated,
				Order: orderDesc,
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.ListFunc = func(repo ghrepo.Interface, filters client.ListFilters, after string, limit int) (*client.DiscussionListResult, error) {
					return sampleResult(), nil
				}
			},
			wantStdout: heredoc.Doc(`
				42	OPEN	Bug report discussion	General	bug	answered	2025-02-28T12:00:00Z
				41	OPEN	Feature request	Ideas			2025-02-20T12:00:00Z
			`),
		},
		{
			name: "json output with next cursor",
			opts: ListOptions{
				State: stateOpen,
				Limit: 30,
				Sort:  sortUpdated,
				Order: orderDesc,
				Exporter: func() cmdutil.Exporter {
					e := cmdutil.NewJSONExporter()
					e.SetFields([]string{"number", "title"})
					return e
				}(),
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.ListFunc = func(repo ghrepo.Interface, filters client.ListFilters, after string, limit int) (*client.DiscussionListResult, error) {
					return &client.DiscussionListResult{
						Discussions: sampleDiscussions(),
						TotalCount:  999,
						NextCursor:  "CURSOR123",
					}, nil
				}
			},
			wantStdout: "{\"discussions\":[{\"number\":42,\"title\":\"Bug report discussion\"},{\"number\":41,\"title\":\"Feature request\"}],\"next\":\"CURSOR123\",\"totalCount\":999}\n",
		},
		{
			name: "json output with current cursor",
			opts: ListOptions{
				State: stateOpen,
				Limit: 30,
				Sort:  sortUpdated,
				Order: orderDesc,
				Exporter: func() cmdutil.Exporter {
					e := cmdutil.NewJSONExporter()
					e.SetFields([]string{"number", "title"})
					return e
				}(),
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.ListFunc = func(repo ghrepo.Interface, filters client.ListFilters, after string, limit int) (*client.DiscussionListResult, error) {
					return &client.DiscussionListResult{
						Discussions: sampleDiscussions(),
						TotalCount:  999,
						Cursor:      "PREV_CURSOR",
					}, nil
				}
			},
			wantStdout: "{\"cursor\":\"PREV_CURSOR\",\"discussions\":[{\"number\":42,\"title\":\"Bug report discussion\"},{\"number\":41,\"title\":\"Feature request\"}],\"totalCount\":999}\n",
		},
		{
			name: "json output omits next when no more pages",
			opts: ListOptions{
				State: stateOpen,
				Limit: 30,
				Sort:  sortUpdated,
				Order: orderDesc,
				Exporter: func() cmdutil.Exporter {
					e := cmdutil.NewJSONExporter()
					e.SetFields([]string{"number", "title"})
					return e
				}(),
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.ListFunc = func(repo ghrepo.Interface, filters client.ListFilters, after string, limit int) (*client.DiscussionListResult, error) {
					return &client.DiscussionListResult{
						Discussions: sampleDiscussions(),
						TotalCount:  2,
					}, nil
				}
			},
			wantStdout: "{\"discussions\":[{\"number\":42,\"title\":\"Bug report discussion\"},{\"number\":41,\"title\":\"Feature request\"}],\"totalCount\":2}\n",
		},
		{
			name: "web mode",
			tty:  true,
			opts: ListOptions{
				State:   stateOpen,
				WebMode: true,
			},
			wantStderr: "Opening https://github.com/OWNER/REPO/discussions in your browser.\n",
			wantBrowse: "https://github.com/OWNER/REPO/discussions?q=is%3Aopen",
		},
		{
			name: "no results",
			tty:  true,
			opts: ListOptions{
				State: stateOpen,
				Limit: 30,
				Sort:  sortUpdated,
				Order: orderDesc,
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.ListFunc = func(repo ghrepo.Interface, filters client.ListFilters, after string, limit int) (*client.DiscussionListResult, error) {
					return &client.DiscussionListResult{}, nil
				}
			},
			wantErr:   "no discussions found in OWNER/REPO",
			wantErrAs: &cmdutil.NoResultsError{},
		},
		{
			name: "category filter",
			tty:  true,
			opts: ListOptions{
				Category: "general",
				State:    stateOpen,
				Limit:    30,
				Sort:     sortUpdated,
				Order:    orderDesc,
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
				m.ListFunc = func(repo ghrepo.Interface, filters client.ListFilters, after string, limit int) (*client.DiscussionListResult, error) {
					assert.Equal(t, "CAT1", filters.CategoryID)
					return &client.DiscussionListResult{
						Discussions: sampleDiscussions()[:1],
						TotalCount:  1,
					}, nil
				}
			},
			wantStdout: heredoc.Doc(`

				Showing 1 of 1 open discussions in OWNER/REPO

				ID   TITLE                  CATEGORY  LABELS  ANSWERED  UPDATED
				#42  Bug report discussion  General   bug     ✓         about 12 hours ago
			`),
		},
		{
			name: "category not found",
			opts: ListOptions{
				Category: "nonexistent",
				State:    stateOpen,
				Limit:    30,
				Sort:     sortUpdated,
				Order:    orderDesc,
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
			},
			wantErr: `unknown category: "nonexistent"`,
		},
		{
			name: "author filter uses search",
			tty:  true,
			opts: ListOptions{
				Author: "monalisa",
				State:  stateOpen,
				Limit:  30,
				Sort:   sortUpdated,
				Order:  orderDesc,
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.SearchFunc = func(repo ghrepo.Interface, filters client.SearchFilters, after string, limit int) (*client.DiscussionListResult, error) {
					assert.Equal(t, "monalisa", filters.Author)
					return &client.DiscussionListResult{
						Discussions: sampleDiscussions()[:1],
						TotalCount:  1,
					}, nil
				}
			},
			wantStdout: heredoc.Doc(`

				Showing 1 of 1 open discussions in OWNER/REPO

				ID   TITLE                  CATEGORY  LABELS  ANSWERED  UPDATED
				#42  Bug report discussion  General   bug     ✓         about 12 hours ago
			`),
		},
		{
			name: "label filter uses search",
			tty:  true,
			opts: ListOptions{
				Labels: []string{"bug", "docs"},
				State:  stateOpen,
				Limit:  30,
				Sort:   sortUpdated,
				Order:  orderDesc,
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.SearchFunc = func(repo ghrepo.Interface, filters client.SearchFilters, after string, limit int) (*client.DiscussionListResult, error) {
					assert.Equal(t, []string{"bug", "docs"}, filters.Labels)
					return &client.DiscussionListResult{
						Discussions: sampleDiscussions()[:1],
						TotalCount:  1,
					}, nil
				}
			},
			wantStdout: heredoc.Doc(`

				Showing 1 of 1 open discussions in OWNER/REPO

				ID   TITLE                  CATEGORY  LABELS  ANSWERED  UPDATED
				#42  Bug report discussion  General   bug     ✓         about 12 hours ago
			`),
		},
		{
			name: "search filter uses search",
			tty:  true,
			opts: ListOptions{
				Search: "some keywords",
				State:  stateOpen,
				Limit:  30,
				Sort:   sortUpdated,
				Order:  orderDesc,
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.SearchFunc = func(repo ghrepo.Interface, filters client.SearchFilters, after string, limit int) (*client.DiscussionListResult, error) {
					assert.Equal(t, "some keywords", filters.Keywords)
					return &client.DiscussionListResult{
						Discussions: sampleDiscussions()[:1],
						TotalCount:  1,
					}, nil
				}
			},
			wantStdout: heredoc.Doc(`

				Showing 1 of 1 open discussions in OWNER/REPO

				ID   TITLE                  CATEGORY  LABELS  ANSWERED  UPDATED
				#42  Bug report discussion  General   bug     ✓         about 12 hours ago
			`),
		},
		{
			name: "after cursor",
			tty:  true,
			opts: ListOptions{
				State: stateOpen,
				Limit: 30,
				Sort:  sortUpdated,
				Order: orderDesc,
				After: "CURSOR_ABC",
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.ListFunc = func(repo ghrepo.Interface, filters client.ListFilters, after string, limit int) (*client.DiscussionListResult, error) {
					assert.Equal(t, "CURSOR_ABC", after)
					return sampleResult(), nil
				}
			},
			wantStdout: heredoc.Doc(`

				Showing 2 of 2 open discussions in OWNER/REPO

				ID   TITLE                  CATEGORY  LABELS  ANSWERED  UPDATED
				#42  Bug report discussion  General   bug     ✓         about 12 hours ago
				#41  Feature request        Ideas                       about 8 days ago
			`),
		},
		{
			name: "after cursor with search",
			tty:  true,
			opts: ListOptions{
				Labels: []string{"bug"},
				State:  stateOpen,
				Limit:  30,
				Sort:   sortUpdated,
				Order:  orderDesc,
				After:  "SEARCH_CURSOR",
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.SearchFunc = func(repo ghrepo.Interface, filters client.SearchFilters, after string, limit int) (*client.DiscussionListResult, error) {
					assert.Equal(t, "SEARCH_CURSOR", after)
					assert.Equal(t, []string{"bug"}, filters.Labels)
					return sampleResult(), nil
				}
			},
			wantStdout: heredoc.Doc(`

				Showing 2 of 2 open discussions in OWNER/REPO

				ID   TITLE                  CATEGORY  LABELS  ANSWERED  UPDATED
				#42  Bug report discussion  General   bug     ✓         about 12 hours ago
				#41  Feature request        Ideas                       about 8 days ago
			`),
		},
		{
			name: "closed state",
			tty:  true,
			opts: ListOptions{
				State: stateClosed,
				Limit: 30,
				Sort:  sortUpdated,
				Order: orderDesc,
			},
			clientStub: func(t *testing.T, m *client.DiscussionClientMock) {
				m.ListFunc = func(repo ghrepo.Interface, filters client.ListFilters, after string, limit int) (*client.DiscussionListResult, error) {
					return &client.DiscussionListResult{
						Discussions: []client.Discussion{
							{
								Number:    10,
								Title:     "Old discussion",
								Closed:    true,
								Category:  client.DiscussionCategory{Name: "General"},
								Labels:    []client.DiscussionLabel{},
								UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
							},
						},
						TotalCount: 1,
					}, nil
				}
			},
			wantStdout: heredoc.Doc(`

				Showing 1 of 1 closed discussions in OWNER/REPO

				ID   TITLE           CATEGORY  LABELS  ANSWERED  UPDATED
				#10  Old discussion  General                     about 1 month ago
			`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)

			opts := tt.opts
			opts.IO = ios
			opts.BaseRepo = func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil }
			opts.Now = fixedTime

			br := &browser.Stub{}
			opts.Browser = br

			if tt.clientStub != nil {
				mock := &client.DiscussionClientMock{}
				tt.clientStub(t, mock)
				opts.Client = func() (client.DiscussionClient, error) { return mock, nil }
			}

			err := listRun(&opts)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				if tt.wantErrAs != nil {
					assert.ErrorAs(t, err, tt.wantErrAs)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			br.Verify(t, tt.wantBrowse)
		})
	}
}

func fixedTime() time.Time {
	return time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
}

func sampleDiscussions() []client.Discussion {
	return []client.Discussion{
		{
			Number: 42,
			Title:  "Bug report discussion",
			URL:    "https://github.com/OWNER/REPO/discussions/42",
			Author: client.DiscussionActor{Login: "monalisa"},
			Category: client.DiscussionCategory{
				ID:   "CAT1",
				Name: "General",
				Slug: "general",
			},
			Labels: []client.DiscussionLabel{
				{ID: "L1", Name: "bug", Color: "d73a4a"},
			},
			Answered:  true,
			UpdatedAt: time.Date(2025, 2, 28, 12, 0, 0, 0, time.UTC),
		},
		{
			Number: 41,
			Title:  "Feature request",
			URL:    "https://github.com/OWNER/REPO/discussions/41",
			Author: client.DiscussionActor{Login: "octocat"},
			Category: client.DiscussionCategory{
				ID:   "CAT2",
				Name: "Ideas",
				Slug: "ideas",
			},
			Labels:    []client.DiscussionLabel{},
			Answered:  false,
			UpdatedAt: time.Date(2025, 2, 20, 12, 0, 0, 0, time.UTC),
		},
	}
}

func sampleResult() *client.DiscussionListResult {
	return &client.DiscussionListResult{
		Discussions: sampleDiscussions(),
		TotalCount:  2,
	}
}

func sampleCategories() []client.DiscussionCategory {
	return []client.DiscussionCategory{
		{ID: "CAT1", Name: "General", Slug: "general", IsAnswerable: true},
		{ID: "CAT2", Name: "Ideas", Slug: "ideas", IsAnswerable: false},
		{ID: "CAT3", Name: "Show and tell", Slug: "show-and-tell", IsAnswerable: false},
	}
}

func TestToFilterState(t *testing.T) {
	tests := []struct {
		input string
		want  *string
	}{
		{input: "open", want: new(client.FilterStateOpen)},
		{input: "closed", want: new(client.FilterStateClosed)},
		{input: "all", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toFilterState(tt.input)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, *tt.want, *got)
			}
		})
	}
}
