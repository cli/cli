package list

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/pkg/cmd/agent-task/capi"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantOpts ListOptions
		wantErr  string
	}{
		{
			name: "no arguments",
			wantOpts: ListOptions{
				Limit: defaultLimit,
			},
		},
		{
			name: "custom limit",
			args: "--limit 15",
			wantOpts: ListOptions{
				Limit: 15,
			},
		},
		{
			name:    "invalid limit",
			args:    "--limit 0",
			wantErr: "invalid limit: 0",
		},
		{
			name:    "negative limit",
			args:    "--limit -5",
			wantErr: "invalid limit: -5",
		},
		{
			name: "web flag",
			args: "--web",
			wantOpts: ListOptions{
				Limit: defaultLimit,
				Web:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			var gotOpts *ListOptions
			cmd := NewCmdList(f, func(opts *ListOptions) error { gotOpts = opts; return nil })

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)

			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOpts.Limit, gotOpts.Limit)
			assert.Equal(t, tt.wantOpts.Web, gotOpts.Web)
		})
	}
}

func Test_listRun(t *testing.T) {
	sampleDate := time.Now().Add(-6 * time.Hour) // 6h ago
	sampleDateString := sampleDate.Format(time.RFC3339)

	tests := []struct {
		name           string
		tty            bool
		capiStubs      func(*testing.T, *capi.CapiClientMock)
		limit          int
		web            bool
		jsonFields     []string
		wantOut        string
		wantErr        error
		wantStderr     string
		wantBrowserURL string
	}{
		{
			name: "viewer-scoped no sessions",
			tty:  true,
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListLatestSessionsForViewerFunc = func(ctx context.Context, limit int) ([]*capi.Session, error) {
					return nil, nil
				}
			},
			wantErr: cmdutil.NewNoResultsError("no agent tasks found"),
		},
		{
			name:  "viewer-scoped respects --limit",
			tty:   true,
			limit: 999,
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListLatestSessionsForViewerFunc = func(ctx context.Context, limit int) ([]*capi.Session, error) {
					assert.Equal(t, 999, limit)
					return nil, nil
				}
			},
			wantErr: cmdutil.NewNoResultsError("no agent tasks found"), // not important
		},
		{
			name: "viewer-scoped single session (tty)",
			tty:  true,
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListLatestSessionsForViewerFunc = func(ctx context.Context, limit int) ([]*capi.Session, error) {
					return []*capi.Session{
						{
							ID:           "id1",
							Name:         "s1",
							State:        "completed",
							CreatedAt:    sampleDate,
							ResourceType: "pull",
							PullRequest: &api.PullRequest{
								Number: 101,
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
						},
					}, nil
				}
			},
			wantOut: heredoc.Doc(`
				Showing 1 session

				SESSION NAME  PULL REQUEST  REPO        SESSION STATE     CREATED
				s1            #101          OWNER/REPO  Ready for review  about 6 hours ago
			`),
		},
		{
			name: "viewer-scoped single session (nontty)",
			tty:  false,
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListLatestSessionsForViewerFunc = func(ctx context.Context, limit int) ([]*capi.Session, error) {
					return []*capi.Session{
						{
							ID:           "id1",
							Name:         "s1",
							State:        "completed",
							ResourceType: "pull",
							CreatedAt:    sampleDate,
							PullRequest: &api.PullRequest{
								Number: 101,
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
						},
					}, nil
				}
			},
			wantOut: "s1\t#101\tOWNER/REPO\tReady for review\t" + sampleDateString + "\n", // header omitted for non-tty
		},
		{
			name: "viewer-scoped many sessions (tty)",
			tty:  true,
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListLatestSessionsForViewerFunc = func(ctx context.Context, limit int) ([]*capi.Session, error) {
					return []*capi.Session{
						{
							ID:           "id1",
							Name:         "s1",
							State:        "completed",
							CreatedAt:    sampleDate,
							ResourceType: "pull",
							PullRequest: &api.PullRequest{
								Number: 101,
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
						},
						{
							ID:           "id2",
							Name:         "s2",
							State:        "failed",
							CreatedAt:    sampleDate,
							ResourceType: "pull",
							PullRequest: &api.PullRequest{
								Number: 102,
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
						},
						{
							ID:           "id3",
							Name:         "s3",
							State:        "in_progress",
							CreatedAt:    sampleDate,
							ResourceType: "pull",
							PullRequest: &api.PullRequest{
								Number: 103,
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
						},
						{
							ID:           "id4",
							Name:         "s4",
							State:        "queued",
							CreatedAt:    sampleDate,
							ResourceType: "pull",
							PullRequest: &api.PullRequest{
								Number: 104,
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
						},
						{
							ID:           "id5",
							Name:         "s5",
							State:        "cancelled",
							CreatedAt:    sampleDate,
							ResourceType: "pull",
							PullRequest: &api.PullRequest{
								Number: 105,
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
						},
						{
							ID:           "id6",
							Name:         "s6",
							State:        "mystery",
							CreatedAt:    sampleDate,
							ResourceType: "pull",
							PullRequest: &api.PullRequest{
								Number: 106,
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
						},
					}, nil
				}
			},
			wantOut: heredoc.Doc(`
				Showing 6 sessions

				SESSION NAME  PULL REQUEST  REPO        SESSION STATE     CREATED
				s1            #101          OWNER/REPO  Ready for review  about 6 hours ago
				s2            #102          OWNER/REPO  Failed            about 6 hours ago
				s3            #103          OWNER/REPO  In progress       about 6 hours ago
				s4            #104          OWNER/REPO  Queued            about 6 hours ago
				s5            #105          OWNER/REPO  Cancelled         about 6 hours ago
				s6            #106          OWNER/REPO  mystery           about 6 hours ago
			`),
		},
		{
			name:           "web mode",
			tty:            true,
			web:            true,
			wantOut:        "",
			wantStderr:     "Opening https://github.com/copilot/agents in your browser.\n",
			wantBrowserURL: "https://github.com/copilot/agents",
		},
		{
			name: "json output",
			tty:  false,
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListLatestSessionsForViewerFunc = func(ctx context.Context, limit int) ([]*capi.Session, error) {
					return []*capi.Session{
						{
							ID:            "abc-123",
							Name:          "s1",
							State:         "completed",
							CreatedAt:     sampleDate,
							LastUpdatedAt: sampleDate,
							CompletedAt:   sampleDate,
							ResourceType:  "pull",
							User:          &api.GitHubUser{Login: "monalisa"},
							PullRequest: &api.PullRequest{
								Number: 101,
								Title:  "Fix login bug",
								State:  "MERGED",
								URL:    "https://github.com/OWNER/REPO/pull/101",
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
						},
					}, nil
				}
			},
			jsonFields: []string{"id", "name", "state", "repository", "user", "pullRequestNumber", "pullRequestUrl", "pullRequestTitle", "pullRequestState"},
			wantOut:    "[{\"id\":\"abc-123\",\"name\":\"s1\",\"pullRequestNumber\":101,\"pullRequestState\":\"MERGED\",\"pullRequestTitle\":\"Fix login bug\",\"pullRequestUrl\":\"https://github.com/OWNER/REPO/pull/101\",\"repository\":\"OWNER/REPO\",\"state\":\"completed\",\"user\":\"monalisa\"}]\n",
		},
		{
			name: "json output with no sessions returns empty array",
			tty:  false,
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListLatestSessionsForViewerFunc = func(ctx context.Context, limit int) ([]*capi.Session, error) {
					return nil, nil
				}
			},
			jsonFields: []string{"id", "name", "state"},
			wantOut:    "[]\n",
		},
		{
			name: "json output with nil pull request",
			tty:  false,
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListLatestSessionsForViewerFunc = func(ctx context.Context, limit int) ([]*capi.Session, error) {
					return []*capi.Session{
						{
							ID:            "abc-456",
							Name:          "s2",
							State:         "in_progress",
							CreatedAt:     sampleDate,
							LastUpdatedAt: sampleDate,
							ResourceType:  "pull",
						},
					}, nil
				}
			},
			jsonFields: []string{"id", "name", "state", "repository", "user", "pullRequestNumber", "pullRequestUrl", "pullRequestTitle", "pullRequestState"},
			wantOut:    "[{\"id\":\"abc-456\",\"name\":\"s2\",\"pullRequestNumber\":null,\"pullRequestState\":null,\"pullRequestTitle\":null,\"pullRequestUrl\":null,\"repository\":null,\"state\":\"in_progress\",\"user\":null}]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capiClientMock := &capi.CapiClientMock{}
			if tt.capiStubs != nil {
				tt.capiStubs(t, capiClientMock)
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)

			var br *browser.Stub
			if tt.web {
				br = &browser.Stub{}
			}

			opts := &ListOptions{
				IO:      ios,
				Limit:   tt.limit,
				Web:     tt.web,
				Browser: br,
				CapiClient: func() (capi.CapiClient, error) {
					if tt.web {
						require.FailNow(t, "CapiClient was called with --web")
					}
					return capiClientMock, nil
				},
			}

			if tt.jsonFields != nil {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields(tt.jsonFields)
				opts.Exporter = exporter
			}

			err := listRun(opts)
			if tt.wantErr != nil {
				assert.Error(t, err)
				require.EqualError(t, err, tt.wantErr.Error())
			} else {
				require.NoError(t, err)
			}
			got := stdout.String()
			require.Equal(t, tt.wantOut, got)
			require.Equal(t, tt.wantStderr, stderr.String())
			if tt.web {
				br.Verify(t, tt.wantBrowserURL)
			}
		})
	}
}
