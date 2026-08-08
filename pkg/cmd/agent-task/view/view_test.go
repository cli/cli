package view

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/agent-task/capi"
	"github.com/cli/cli/v2/pkg/cmd/agent-task/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name         string
		tty          bool
		args         string
		wantOpts     ViewOptions
		wantBaseRepo ghrepo.Interface
		wantErr      string
	}{
		{
			name:     "no arg tty",
			tty:      true,
			args:     "",
			wantOpts: ViewOptions{},
		},
		{
			name: "session ID arg tty",
			tty:  true,
			args: "00000000-0000-0000-0000-000000000000",
			wantOpts: ViewOptions{
				SelectorArg: "00000000-0000-0000-0000-000000000000",
				SessionID:   "00000000-0000-0000-0000-000000000000",
			},
		},
		{
			name: "PR agent-session URL arg tty",
			tty:  true,
			args: "https://github.com/OWNER/REPO/pull/101/agent-sessions/00000000-0000-0000-0000-000000000000",
			wantOpts: ViewOptions{
				SelectorArg: "https://github.com/OWNER/REPO/pull/101/agent-sessions/00000000-0000-0000-0000-000000000000",
				SessionID:   "00000000-0000-0000-0000-000000000000",
			},
		},
		{
			name: "non-session ID arg tty",
			tty:  true,
			args: "some-arg",
			wantOpts: ViewOptions{
				SelectorArg: "some-arg",
			},
		},
		{
			name:    "session ID required if non-tty",
			tty:     false,
			args:    "some-arg",
			wantErr: "session ID is required when not running interactively",
		},
		{
			name:         "repo override",
			tty:          true,
			args:         "some-arg -R OWNER/REPO",
			wantBaseRepo: ghrepo.New("OWNER", "REPO"),
			wantOpts: ViewOptions{
				SelectorArg: "some-arg",
			},
		},
		{
			name: "with --log",
			tty:  true,
			args: "some-arg --log",
			wantOpts: ViewOptions{
				SelectorArg: "some-arg",
				Log:         true,
			},
		},
		{
			name: "with --log and --follow",
			tty:  true,
			args: "some-arg --log --follow",
			wantOpts: ViewOptions{
				SelectorArg: "some-arg",
				Log:         true,
				Follow:      true,
			},
		},
		{
			name:    "--follow requires --log",
			tty:     true,
			args:    "some-arg --follow",
			wantErr: "--log is required when providing --follow",
		},
		{
			name: "web mode",
			tty:  true,
			args: "some-arg -w",
			wantOpts: ViewOptions{
				SelectorArg: "some-arg",
				Web:         true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)

			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			var gotOpts *ViewOptions
			cmd := NewCmdView(f, func(opts *ViewOptions) error { gotOpts = opts; return nil })

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
			assert.Equal(t, tt.wantOpts.SelectorArg, gotOpts.SelectorArg)
			assert.Equal(t, tt.wantOpts.SessionID, gotOpts.SessionID)

			if tt.wantBaseRepo != nil {
				baseRepo, err := gotOpts.BaseRepo()
				require.NoError(t, err)
				assert.True(t, ghrepo.IsSame(tt.wantBaseRepo, baseRepo))
			}
		})
	}
}

func Test_viewRun(t *testing.T) {
	sampleDate := time.Now().Add(-6 * time.Hour) // 6h ago
	sampleCompletedAt := sampleDate.Add(5 * time.Minute)

	tests := []struct {
		name             string
		tty              bool
		opts             ViewOptions
		promptStubs      func(*testing.T, *prompter.MockPrompter)
		capiStubs        func(*testing.T, *capi.CapiClientMock)
		logRendererStubs func(*testing.T, *shared.LogRendererMock)
		jsonFields       []string
		wantOut          string
		wantErr          error
		wantStderr       string
		wantBrowserURL   string
	}{
		{
			name: "with session id, not found (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, _ string) (*capi.Session, error) {
					return nil, capi.ErrSessionNotFound
				}
			},
			wantStderr: "session not found\n",
			wantErr:    cmdutil.SilentError,
		},
		{
			name: "with session id, api error (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, _ string) (*capi.Session, error) {
					return nil, errors.New("some error")
				}
			},
			wantErr: errors.New("some error"),
		},
		{
			name: "with session id, success, with pr and user data (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						State:           "completed",
						Name:            "session one",
						CreatedAt:       sampleDate,
						CompletedAt:     sampleCompletedAt,
						PremiumRequests: 1.5,
						PullRequest: &api.PullRequest{
							Title:  "fix something",
							Number: 101,
							URL:    "https://github.com/OWNER/REPO/pull/101",
							Repository: &api.PRRepository{
								NameWithOwner: "OWNER/REPO",
							},
						},
						User: &api.GitHubUser{
							Login: "octocat",
						},
					}, nil
				}
			},
			wantOut: heredoc.Doc(`
				Ready for review • session one
				Started on behalf of octocat about 6 hours ago
				Used 1.5 premium request(s) • Duration 5m0s

				OWNER/REPO#101 • fix something

				For detailed session logs, try:
				gh agent-task view 'some-session-id' --log

				View this session on GitHub:
				https://github.com/OWNER/REPO/pull/101/agent-sessions/some-session-id
			`),
		},
		{
			// The user data should always be there, but we need to cover the code path.
			name: "with session id, success, without user data (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						State:           "completed",
						Name:            "session one",
						CreatedAt:       sampleDate,
						CompletedAt:     sampleCompletedAt,
						PremiumRequests: 1.5,
						PullRequest: &api.PullRequest{
							Title:  "fix something",
							Number: 101,
							URL:    "https://github.com/OWNER/REPO/pull/101",
							Repository: &api.PRRepository{
								NameWithOwner: "OWNER/REPO",
							},
						},
					}, nil
				}
			},
			wantOut: heredoc.Doc(`
				Ready for review • session one
				Started about 6 hours ago
				Used 1.5 premium request(s) • Duration 5m0s

				OWNER/REPO#101 • fix something

				For detailed session logs, try:
				gh agent-task view 'some-session-id' --log

				View this session on GitHub:
				https://github.com/OWNER/REPO/pull/101/agent-sessions/some-session-id
			`),
		},
		{
			// This can happen when the session is just created and a PR is not yet available for it.
			name: "with session id, success, without pr data (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						State:           "completed",
						Name:            "session one",
						CreatedAt:       sampleDate,
						CompletedAt:     sampleCompletedAt,
						PremiumRequests: 1.5,
						User: &api.GitHubUser{
							Login: "octocat",
						},
					}, nil
				}
			},
			wantOut: heredoc.Doc(`
				Ready for review • session one
				Started on behalf of octocat about 6 hours ago
				Used 1.5 premium request(s) • Duration 5m0s

				For detailed session logs, try:
				gh agent-task view 'some-session-id' --log
			`),
		},
		{
			// The user data should always be there, but we need to cover the code path.
			name: "with session id, success, without pr nor user data (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						State:           "completed",
						Name:            "session one",
						CreatedAt:       sampleDate,
						CompletedAt:     sampleCompletedAt,
						PremiumRequests: 1.5,
					}, nil
				}
			},
			wantOut: heredoc.Doc(`
				Ready for review • session one
				Started about 6 hours ago
				Used 1.5 premium request(s) • Duration 5m0s

				For detailed session logs, try:
				gh agent-task view 'some-session-id' --log
			`),
		},
		{
			name: "with session id, success, with zero premium requests (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						State:           "completed",
						Name:            "session one",
						CreatedAt:       sampleDate,
						CompletedAt:     sampleCompletedAt,
						PremiumRequests: 0,
						PullRequest: &api.PullRequest{
							Title:  "fix something",
							Number: 101,
							URL:    "https://github.com/OWNER/REPO/pull/101",
							Repository: &api.PRRepository{
								NameWithOwner: "OWNER/REPO",
							},
						},
						User: &api.GitHubUser{
							Login: "octocat",
						},
					}, nil
				}
			},
			wantOut: heredoc.Doc(`
				Ready for review • session one
				Started on behalf of octocat about 6 hours ago
				Used 0 premium request(s) • Duration 5m0s

				OWNER/REPO#101 • fix something

				For detailed session logs, try:
				gh agent-task view 'some-session-id' --log

				View this session on GitHub:
				https://github.com/OWNER/REPO/pull/101/agent-sessions/some-session-id
			`),
		},
		{
			name: "with session id, success, duration not available (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						State:           "in_progress",
						Name:            "session one",
						CreatedAt:       sampleDate,
						PremiumRequests: 1.5,
						PullRequest: &api.PullRequest{
							Title:  "fix something",
							Number: 101,
							URL:    "https://github.com/OWNER/REPO/pull/101",
							Repository: &api.PRRepository{
								NameWithOwner: "OWNER/REPO",
							},
						},
						User: &api.GitHubUser{
							Login: "octocat",
						},
					}, nil
				}
			},
			wantOut: heredoc.Doc(`
				In progress • session one
				Started on behalf of octocat about 6 hours ago
				Used 1.5 premium request(s)

				OWNER/REPO#101 • fix something

				For detailed session logs, try:
				gh agent-task view 'some-session-id' --log

				View this session on GitHub:
				https://github.com/OWNER/REPO/pull/101/agent-sessions/some-session-id
			`),
		},
		{
			name: "with session id, success, session has error (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						State:           "failed",
						Name:            "session one",
						CreatedAt:       sampleDate,
						PremiumRequests: 1.5,
						Error: &capi.SessionError{
							Message: "blah blah",
						},
						PullRequest: &api.PullRequest{
							Title:  "fix something",
							Number: 101,
							URL:    "https://github.com/OWNER/REPO/pull/101",
							Repository: &api.PRRepository{
								NameWithOwner: "OWNER/REPO",
							},
						},
						User: &api.GitHubUser{
							Login: "octocat",
						},
					}, nil
				}
			},
			wantOut: heredoc.Doc(`
				Failed • session one
				Started on behalf of octocat about 6 hours ago
				Used 1.5 premium request(s)

				OWNER/REPO#101 • fix something

				X blah blah

				For detailed session logs, try:
				gh agent-task view 'some-session-id' --log

				View this session on GitHub:
				https://github.com/OWNER/REPO/pull/101/agent-sessions/some-session-id
			`),
		},
		{
			name: "with session id, success, session has error with workflow id (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						State:           "failed",
						Name:            "session one",
						CreatedAt:       sampleDate,
						PremiumRequests: 1.5,
						WorkflowRunID:   9999,
						Error: &capi.SessionError{
							Message: "blah blah",
						},
						PullRequest: &api.PullRequest{
							Title:  "fix something",
							Number: 101,
							URL:    "https://github.com/OWNER/REPO/pull/101",
							Repository: &api.PRRepository{
								NameWithOwner: "OWNER/REPO",
							},
						},
						User: &api.GitHubUser{
							Login: "octocat",
						},
					}, nil
				}
			},
			wantOut: heredoc.Doc(`
				Failed • session one
				Started on behalf of octocat about 6 hours ago
				Used 1.5 premium request(s)

				OWNER/REPO#101 • fix something

				X blah blah
				https://github.com/OWNER/REPO/actions/runs/9999

				For detailed session logs, try:
				gh agent-task view 'some-session-id' --log

				View this session on GitHub:
				https://github.com/OWNER/REPO/pull/101/agent-sessions/some-session-id
			`),
		},
		{
			name: "with session id, not found, web mode (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
				Web:         true,
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, _ string) (*capi.Session, error) {
					return nil, capi.ErrSessionNotFound
				}
			},
			wantStderr: "session not found\n",
			wantErr:    cmdutil.SilentError,
		},
		{
			name: "with session id, without pr data, web mode (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
				Web:         true,
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						State:           "completed",
						Name:            "session one",
						CreatedAt:       sampleDate,
						CompletedAt:     sampleCompletedAt,
						PremiumRequests: 1.5,
						// User data is irrelevant in this case
					}, nil
				}
			},
			wantBrowserURL: "https://github.com/copilot/agents",
			wantStderr:     "Opening https://github.com/copilot/agents in your browser.\n",
		},
		{
			name: "with session id, with pr data, web mode (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
				Web:         true,
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						State:           "completed",
						Name:            "session one",
						CreatedAt:       sampleDate,
						CompletedAt:     sampleCompletedAt,
						PremiumRequests: 1.5,
						PullRequest: &api.PullRequest{
							Title:  "fix something",
							Number: 101,
							URL:    "https://github.com/OWNER/REPO/pull/101",
							Repository: &api.PRRepository{
								NameWithOwner: "OWNER/REPO",
							},
						},
						// User data is irrelevant in this case
					}, nil
				}
			},
			wantBrowserURL: "https://github.com/OWNER/REPO/pull/101/agent-sessions/some-session-id",
			wantStderr:     "Opening https://github.com/OWNER/REPO/pull/101/agent-sessions/some-session-id in your browser.\n",
		},
		{
			name: "with pr number, api error (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "101",
				Finder: prShared.NewMockFinder(
					"101",
					&api.PullRequest{
						FullDatabaseID: "999999",
						URL:            "https://github.com/OWNER/REPO/pull/101",
					},
					ghrepo.New("OWNER", "REPO"),
				),
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListSessionsByResourceIDFunc = func(_ context.Context, _ string, _ int64, _ int) ([]*capi.Session, error) {
					return nil, errors.New("some error")
				}
			},
			wantErr: errors.New("failed to list sessions for pull request: some error"),
		},
		{
			name: "with pr reference, unsupported hostname (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "OWNER/REPO#101",
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.NewWithHost("OWNER", "REPO", "foo.com"), nil
				},
			},
			wantErr: errors.New("agent tasks are not supported on this host: foo.com"),
		},
		{
			name: "with pr reference, api error when fetching pr database ID (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "OWNER/REPO#101",
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetPullRequestDatabaseIDFunc = func(_ context.Context, _ string, _ string, _ string, _ int) (int64, string, error) {
					return 0, "", errors.New("some error")
				}
			},
			wantErr: errors.New("failed to fetch pull request: some error"),
		},
		{
			name: "with pr reference, api error when fetching session (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "OWNER/REPO#101",
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetPullRequestDatabaseIDFunc = func(_ context.Context, _ string, _ string, _ string, _ int) (int64, string, error) {
					return 999999, "some-url", nil
				}
				m.ListSessionsByResourceIDFunc = func(_ context.Context, _ string, _ int64, _ int) ([]*capi.Session, error) {
					return nil, errors.New("some error")
				}
			},
			wantErr: errors.New("failed to list sessions for pull request: some error"),
		},
		{
			name: "with pr number, success, single session (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "101",
				Finder: prShared.NewMockFinder(
					"101",
					&api.PullRequest{
						FullDatabaseID: "999999",
						URL:            "https://github.com/OWNER/REPO/pull/101",
					},
					ghrepo.New("OWNER", "REPO"),
				),
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListSessionsByResourceIDFunc = func(_ context.Context, resourceType string, resourceID int64, limit int) ([]*capi.Session, error) {
					assert.Equal(t, "pull", resourceType)
					assert.Equal(t, int64(999999), resourceID)
					assert.Equal(t, defaultLimit, limit)
					return []*capi.Session{
						{
							ID:            "some-session-id",
							Name:          "session one",
							State:         "completed",
							LastUpdatedAt: sampleCompletedAt,
							// Rest of the fields are not not meant to be used or relied upon
						},
					}, nil
				}

				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						State:           "completed",
						Name:            "session one",
						CreatedAt:       sampleDate,
						CompletedAt:     sampleCompletedAt,
						PremiumRequests: 1.5,
						PullRequest: &api.PullRequest{
							Title:  "fix something",
							Number: 101,
							URL:    "https://github.com/OWNER/REPO/pull/101",
							Repository: &api.PRRepository{
								NameWithOwner: "OWNER/REPO",
							},
						},
						User: &api.GitHubUser{
							Login: "octocat",
						},
					}, nil
				}
			},
			wantOut: heredoc.Doc(`
				Ready for review • session one
				Started on behalf of octocat about 6 hours ago
				Used 1.5 premium request(s) • Duration 5m0s

				OWNER/REPO#101 • fix something

				For detailed session logs, try:
				gh agent-task view 'some-session-id' --log

				View this session on GitHub:
				https://github.com/OWNER/REPO/pull/101/agent-sessions/some-session-id
			`),
		},
		{
			name: "with pr number, success, multiple sessions (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "101",
				Finder: prShared.NewMockFinder(
					"101",
					&api.PullRequest{
						FullDatabaseID: "999999",
						URL:            "https://github.com/OWNER/REPO/pull/101",
					},
					ghrepo.New("OWNER", "REPO"),
				),
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListSessionsByResourceIDFunc = func(_ context.Context, resourceType string, resourceID int64, limit int) ([]*capi.Session, error) {
					assert.Equal(t, "pull", resourceType)
					assert.Equal(t, int64(999999), resourceID)
					assert.Equal(t, defaultLimit, limit)
					return []*capi.Session{
						{
							ID:            "some-session-id",
							Name:          "session one",
							State:         "completed",
							LastUpdatedAt: sampleCompletedAt,
							// Rest of the fields are not not meant to be used or relied upon
						},
						{
							ID:            "some-other-session-id",
							Name:          "session two",
							State:         "completed",
							LastUpdatedAt: sampleCompletedAt,
							// Rest of the fields are not not meant to be used or relied upon
						},
					}, nil
				}

				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						Name:            "session one",
						State:           "completed",
						CreatedAt:       sampleDate,
						LastUpdatedAt:   sampleCompletedAt,
						CompletedAt:     sampleCompletedAt,
						PremiumRequests: 1.5,
						PullRequest: &api.PullRequest{
							Title:  "fix something",
							Number: 101,
							URL:    "https://github.com/OWNER/REPO/pull/101",
							Repository: &api.PRRepository{
								NameWithOwner: "OWNER/REPO",
							},
						},
						User: &api.GitHubUser{
							Login: "octocat",
						},
					}, nil
				}
			},
			promptStubs: func(t *testing.T, pm *prompter.MockPrompter) {
				pm.RegisterSelect(
					"Select a session",
					[]string{
						"✓ session one • updated about 5 hours ago",
						"✓ session two • updated about 5 hours ago",
					},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ session one • updated about 5 hours ago")
					},
				)
			},
			wantOut: heredoc.Doc(`
				Ready for review • session one
				Started on behalf of octocat about 6 hours ago
				Used 1.5 premium request(s) • Duration 5m0s

				OWNER/REPO#101 • fix something

				For detailed session logs, try:
				gh agent-task view 'some-session-id' --log

				View this session on GitHub:
				https://github.com/OWNER/REPO/pull/101/agent-sessions/some-session-id
			`),
		},
		{
			name: "with pr reference, success, multiple sessions (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "OWNER/REPO#101",
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetPullRequestDatabaseIDFunc = func(_ context.Context, hostname string, owner string, repo string, number int) (int64, string, error) {
					assert.Equal(t, "github.com", hostname)
					assert.Equal(t, "OWNER", owner)
					assert.Equal(t, "REPO", repo)
					assert.Equal(t, 101, number)
					return 999999, "https://github.com/OWNER/REPO/pull/101", nil
				}
				m.ListSessionsByResourceIDFunc = func(_ context.Context, resourceType string, resourceID int64, limit int) ([]*capi.Session, error) {
					assert.Equal(t, "pull", resourceType)
					assert.Equal(t, int64(999999), resourceID)
					assert.Equal(t, defaultLimit, limit)
					return []*capi.Session{
						{
							ID:            "some-session-id",
							Name:          "session one",
							State:         "completed",
							LastUpdatedAt: sampleCompletedAt,
							// Rest of the fields are not not meant to be used or relied upon
						},
						{
							ID:            "some-other-session-id",
							Name:          "session two",
							State:         "completed",
							LastUpdatedAt: sampleCompletedAt,
							// Rest of the fields are not not meant to be used or relied upon
						},
					}, nil
				}

				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						Name:            "session one",
						State:           "completed",
						CreatedAt:       sampleDate,
						CompletedAt:     sampleCompletedAt,
						LastUpdatedAt:   sampleCompletedAt,
						PremiumRequests: 1.5,
						PullRequest: &api.PullRequest{
							Title:  "fix something",
							Number: 101,
							URL:    "https://github.com/OWNER/REPO/pull/101",
							Repository: &api.PRRepository{
								NameWithOwner: "OWNER/REPO",
							},
						},
						User: &api.GitHubUser{
							Login: "octocat",
						},
					}, nil
				}
			},
			promptStubs: func(t *testing.T, pm *prompter.MockPrompter) {
				pm.RegisterSelect(
					"Select a session",
					[]string{
						"✓ session one • updated about 5 hours ago",
						"✓ session two • updated about 5 hours ago",
					},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ session one • updated about 5 hours ago")
					},
				)
			},
			wantOut: heredoc.Doc(`
				Ready for review • session one
				Started on behalf of octocat about 6 hours ago
				Used 1.5 premium request(s) • Duration 5m0s

				OWNER/REPO#101 • fix something

				For detailed session logs, try:
				gh agent-task view 'some-session-id' --log

				View this session on GitHub:
				https://github.com/OWNER/REPO/pull/101/agent-sessions/some-session-id
			`),
		},
		{
			name: "with pr number, api error, web mode (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "101",
				Finder: prShared.NewMockFinder(
					"101",
					&api.PullRequest{
						FullDatabaseID: "999999",
						URL:            "https://github.com/OWNER/REPO/pull/101",
					},
					ghrepo.New("OWNER", "REPO"),
				),
				Web: true,
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListSessionsByResourceIDFunc = func(_ context.Context, _ string, _ int64, _ int) ([]*capi.Session, error) {
					return nil, errors.New("some error")
				}
			},
			wantErr: errors.New("failed to list sessions for pull request: some error"),
		},
		{
			name: "with pr number, single session, web mode (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "101",
				Finder: prShared.NewMockFinder(
					"101",
					&api.PullRequest{
						FullDatabaseID: "999999",
						URL:            "https://github.com/OWNER/REPO/pull/101",
					},
					ghrepo.New("OWNER", "REPO"),
				),
				Web: true,
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListSessionsByResourceIDFunc = func(_ context.Context, resourceType string, resourceID int64, limit int) ([]*capi.Session, error) {
					assert.Equal(t, "pull", resourceType)
					assert.Equal(t, int64(999999), resourceID)
					assert.Equal(t, defaultLimit, limit)
					return []*capi.Session{
						{
							ID:              "some-session-id",
							State:           "completed",
							Name:            "session one",
							CreatedAt:       sampleDate,
							CompletedAt:     sampleCompletedAt,
							PremiumRequests: 1.5,
							PullRequest: &api.PullRequest{
								Title:  "fix something",
								Number: 101,
								URL:    "https://github.com/OWNER/REPO/pull/101",
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
							// User data is irrelevant in this case
						},
					}, nil
				}
			},
			wantBrowserURL: "https://github.com/OWNER/REPO/pull/101/agent-sessions",
			wantStderr:     "Opening https://github.com/OWNER/REPO/pull/101/agent-sessions in your browser.\n",
		},
		{
			name: "with pr number, multiple sessions, web mode (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "101",
				Finder: prShared.NewMockFinder(
					"101",
					&api.PullRequest{
						FullDatabaseID: "999999",
						URL:            "https://github.com/OWNER/REPO/pull/101",
					},
					ghrepo.New("OWNER", "REPO"),
				),
				Web: true,
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.ListSessionsByResourceIDFunc = func(_ context.Context, resourceType string, resourceID int64, limit int) ([]*capi.Session, error) {
					assert.Equal(t, "pull", resourceType)
					assert.Equal(t, int64(999999), resourceID)
					assert.Equal(t, defaultLimit, limit)
					return []*capi.Session{
						{
							ID:              "some-session-id",
							Name:            "session one",
							State:           "completed",
							CreatedAt:       sampleDate,
							CompletedAt:     sampleCompletedAt,
							PremiumRequests: 1.5,
							PullRequest: &api.PullRequest{
								Title:  "fix something",
								Number: 101,
								URL:    "https://github.com/OWNER/REPO/pull/101",
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
							// User data is irrelevant in this case
						},
						{
							ID:              "some-other-session-id",
							Name:            "session two",
							State:           "completed",
							CreatedAt:       sampleDate,
							CompletedAt:     sampleCompletedAt,
							PremiumRequests: 1.5,
							PullRequest: &api.PullRequest{
								Title:  "fix something",
								Number: 101,
								URL:    "https://github.com/OWNER/REPO/pull/101",
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
							// User data is irrelevant in this case
						},
					}, nil
				}
			},
			wantBrowserURL: "https://github.com/OWNER/REPO/pull/101/agent-sessions",
			wantStderr:     "Opening https://github.com/OWNER/REPO/pull/101/agent-sessions in your browser.\n",
		},
		{
			name: "with pr reference, multiple sessions, web mode (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "OWNER/REPO#101",
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				Web: true,
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetPullRequestDatabaseIDFunc = func(_ context.Context, hostname string, owner string, repo string, number int) (int64, string, error) {
					assert.Equal(t, "github.com", hostname)
					assert.Equal(t, "OWNER", owner)
					assert.Equal(t, "REPO", repo)
					assert.Equal(t, 101, number)
					return 999999, "https://github.com/OWNER/REPO/pull/101", nil
				}
				m.ListSessionsByResourceIDFunc = func(_ context.Context, resourceType string, resourceID int64, limit int) ([]*capi.Session, error) {
					assert.Equal(t, "pull", resourceType)
					assert.Equal(t, int64(999999), resourceID)
					assert.Equal(t, defaultLimit, limit)
					return []*capi.Session{
						{
							ID:              "some-session-id",
							Name:            "session one",
							State:           "completed",
							CreatedAt:       sampleDate,
							CompletedAt:     sampleCompletedAt,
							PremiumRequests: 1.5,
							PullRequest: &api.PullRequest{
								Title:  "fix something",
								Number: 101,
								URL:    "https://github.com/OWNER/REPO/pull/101",
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
							// User data is irrelevant in this case
						},
						{
							ID:              "some-other-session-id",
							Name:            "session two",
							State:           "completed",
							CreatedAt:       sampleDate,
							CompletedAt:     sampleCompletedAt,
							PremiumRequests: 1.5,
							PullRequest: &api.PullRequest{
								Title:  "fix something",
								Number: 101,
								URL:    "https://github.com/OWNER/REPO/pull/101",
								Repository: &api.PRRepository{
									NameWithOwner: "OWNER/REPO",
								},
							},
							// User data is irrelevant in this case
						},
					}, nil
				}
			},
			wantBrowserURL: "https://github.com/OWNER/REPO/pull/101/agent-sessions",
			wantStderr:     "Opening https://github.com/OWNER/REPO/pull/101/agent-sessions in your browser.\n",
		},
		{
			name: "with log (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
				Log:         true,
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						State:           "completed",
						Name:            "session one",
						CreatedAt:       sampleDate,
						CompletedAt:     sampleCompletedAt,
						PremiumRequests: 1.5,
						User: &api.GitHubUser{
							Login: "octocat",
						},
					}, nil
				}
				m.GetSessionLogsFunc = func(_ context.Context, id string) ([]byte, error) {
					assert.Equal(t, "some-session-id", id)
					return []byte("<raw-logs>"), nil
				}
			},
			logRendererStubs: func(t *testing.T, m *shared.LogRendererMock) {
				m.RenderFunc = func(raw []byte, w io.Writer, ios *iostreams.IOStreams) (bool, error) {
					w.Write([]byte("(rendered:) " + string(raw) + "\n"))
					return false, nil
				}
			},
			wantOut: heredoc.Doc(`
				(rendered:) <raw-logs>
			`),
		},
		{
			name: "with log and follow (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
				Log:         true,
				Follow:      true,
				Sleep:       func(_ time.Duration) {},
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					assert.Equal(t, "some-session-id", id)
					return &capi.Session{
						ID:              "some-session-id",
						State:           "completed",
						Name:            "session one",
						CreatedAt:       sampleDate,
						CompletedAt:     sampleCompletedAt,
						PremiumRequests: 1.5,
						User: &api.GitHubUser{
							Login: "octocat",
						},
					}, nil
				}

				var count int
				m.GetSessionLogsFunc = func(_ context.Context, id string) ([]byte, error) {
					assert.Equal(t, "some-session-id", id)

					count++
					require.Less(t, count, 3, "too many calls to fetch logs")
					if count == 1 {
						return []byte("<raw-logs-one>"), nil
					}
					return []byte("<raw-logs-two>"), nil
				}
			},
			logRendererStubs: func(t *testing.T, m *shared.LogRendererMock) {
				m.FollowFunc = func(fetcher func() ([]byte, error), w io.Writer, ios *iostreams.IOStreams) error {
					raw, err := fetcher()
					require.NoError(t, err)
					w.Write([]byte("(rendered:) " + string(raw) + "\n"))

					raw, err = fetcher()
					require.NoError(t, err)
					w.Write([]byte("(rendered:) " + string(raw) + "\n"))
					return nil
				}
			},
			wantOut: heredoc.Doc(`
				(rendered:) <raw-logs-one>
				(rendered:) <raw-logs-two>
			`),
		},
		{
			name: "json output (tty)",
			tty:  true,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					return &capi.Session{
						ID:            "some-session-id",
						Name:          "Fix login bug",
						State:         "completed",
						CreatedAt:     sampleDate,
						LastUpdatedAt: sampleDate,
						CompletedAt:   sampleCompletedAt,
						ResourceType:  "pull",
						PullRequest: &api.PullRequest{
							Number: 42,
							URL:    "https://github.com/OWNER/REPO/pull/42",
							Title:  "Fix login bug",
							State:  "MERGED",
							Repository: &api.PRRepository{
								NameWithOwner: "OWNER/REPO",
							},
						},
						User: &api.GitHubUser{
							Login: "testuser",
						},
					}, nil
				}
			},
			wantOut:    "{\"id\":\"some-session-id\",\"name\":\"Fix login bug\",\"pullRequestNumber\":42,\"pullRequestState\":\"MERGED\",\"pullRequestTitle\":\"Fix login bug\",\"pullRequestUrl\":\"https://github.com/OWNER/REPO/pull/42\",\"repository\":\"OWNER/REPO\",\"state\":\"completed\",\"user\":\"testuser\"}\n",
			jsonFields: []string{"id", "name", "state", "repository", "user", "pullRequestNumber", "pullRequestUrl", "pullRequestTitle", "pullRequestState"},
		},
		{
			name: "json output with nil pull request",
			tty:  false,
			opts: ViewOptions{
				SelectorArg: "some-session-id",
				SessionID:   "some-session-id",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.GetSessionFunc = func(_ context.Context, id string) (*capi.Session, error) {
					return &capi.Session{
						ID:            "some-session-id",
						Name:          "New task",
						State:         "in_progress",
						CreatedAt:     sampleDate,
						LastUpdatedAt: sampleDate,
						ResourceType:  "pull",
					}, nil
				}
			},
			wantOut:    "{\"id\":\"some-session-id\",\"name\":\"New task\",\"pullRequestNumber\":null,\"pullRequestUrl\":null,\"repository\":null,\"state\":\"in_progress\",\"user\":null}\n",
			jsonFields: []string{"id", "name", "state", "repository", "user", "pullRequestNumber", "pullRequestUrl"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capiClientMock := &capi.CapiClientMock{}
			if tt.capiStubs != nil {
				tt.capiStubs(t, capiClientMock)
			}

			prompter := prompter.NewMockPrompter(t)
			if tt.promptStubs != nil {
				tt.promptStubs(t, prompter)
			}

			logRenderer := &shared.LogRendererMock{}
			if tt.logRendererStubs != nil {
				tt.logRendererStubs(t, logRenderer)
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)

			browser := &browser.Stub{}

			opts := tt.opts
			opts.IO = ios
			opts.Prompter = prompter
			opts.Browser = browser
			opts.CapiClient = func() (capi.CapiClient, error) {
				return capiClientMock, nil
			}
			opts.LogRenderer = func() shared.LogRenderer {
				return logRenderer
			}

			if tt.jsonFields != nil {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields(tt.jsonFields)
				opts.Exporter = exporter
			}

			err := viewRun(&opts)
			if tt.wantErr != nil {
				assert.Error(t, err)
				require.EqualError(t, err, tt.wantErr.Error())
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantBrowserURL, browser.BrowsedURL())
		})
	}
}
