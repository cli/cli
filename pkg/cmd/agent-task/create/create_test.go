package create

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cenkalti/backoff/v4"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/agent-task/capi"
	"github.com/cli/cli/v2/pkg/cmd/agent-task/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		tty      bool
		wantOpts *CreateOptions
		wantErr  string
	}{
		{
			name: "no args nor file returns no error (prompting path)",
			tty:  true,
			wantOpts: &CreateOptions{
				ProblemStatement:     "",
				ProblemStatementFile: "",
			},
		},
		{
			name: "arg only success",
			args: "'task description from args'",
			wantOpts: &CreateOptions{
				ProblemStatement:     "task description from args",
				ProblemStatementFile: "",
			},
		},
		{
			name:    "empty arg",
			args:    "''",
			wantErr: "task description cannot be empty",
		},
		{
			name:    "whitespace arg",
			args:    "'   '",
			wantErr: "task description cannot be empty",
		},
		{
			name:    "whitespace and newline arg",
			args:    "'\n'",
			wantErr: "task description cannot be empty",
		},
		{
			name:    "mutually exclusive arg and file",
			args:    "'some task inline' -F foo.md",
			wantErr: "only one of -F or arg can be provided",
		},
		{
			name: "base branch sets baseBranch field",
			args: "'task description' -b feature",
			wantOpts: &CreateOptions{
				ProblemStatement:     "task description",
				ProblemStatementFile: "",
				BaseBranch:           "feature",
			},
		},
		{
			name: "with --follow",
			args: "'task description from args' --follow",
			wantOpts: &CreateOptions{
				ProblemStatement:     "task description from args",
				ProblemStatementFile: "",
				Follow:               true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, _, _ := iostreams.Test()
			if tt.tty {
				ios.SetStdinTTY(true)
				ios.SetStdoutTTY(true)
				ios.SetStderrTTY(true)
			}
			f := &cmdutil.Factory{IOStreams: ios}

			var gotOpts *CreateOptions
			cmd := NewCmdCreate(f, func(o *CreateOptions) error {
				gotOpts = o
				return nil
			})

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)
			cmd.SetIn(stdin)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			if tt.wantOpts != nil {
				require.Equal(t, tt.wantOpts.ProblemStatement, gotOpts.ProblemStatement)
				require.Equal(t, tt.wantOpts.ProblemStatementFile, gotOpts.ProblemStatementFile)
				require.Equal(t, tt.wantOpts.BaseBranch, gotOpts.BaseBranch)
			}
		})
	}
}

func Test_createRun(t *testing.T) {
	tmpDir := t.TempDir()
	taskDescFile := filepath.Join(tmpDir, "task-description.md")
	emptyTaskDescFile := filepath.Join(tmpDir, "empty-task-description.md")
	require.NoError(t, os.WriteFile(taskDescFile, []byte("task description from file"), 0600))
	require.NoError(t, os.WriteFile(emptyTaskDescFile, []byte("  \n\n"), 0600))

	sampleDateString := "2025-08-29T00:00:00Z"
	sampleDate, err := time.Parse(time.RFC3339, sampleDateString)
	require.NoError(t, err)

	createdJobSuccess := capi.Job{
		ID:        "job123",
		SessionID: "sess1",
		Actor: &capi.JobActor{
			ID:    1,
			Login: "octocat",
		},
		CreatedAt: sampleDate,
		UpdatedAt: sampleDate,
	}
	createdJobSuccessWithPR := capi.Job{
		ID:        "job123",
		SessionID: "sess1",
		Actor: &capi.JobActor{
			ID:    1,
			Login: "octocat",
		},
		CreatedAt: sampleDate,
		UpdatedAt: sampleDate,
		PullRequest: &capi.JobPullRequest{
			ID:     101,
			Number: 42,
		},
	}

	tests := []struct {
		name             string
		isTTY            bool
		opts             *CreateOptions // input options (IO & BackOff set later)
		capiStubs        func(*testing.T, *capi.CapiClientMock)
		logRendererStubs func(*testing.T, *shared.LogRendererMock)
		wantStdout       string
		wantStdErr       string
		wantErr          string
		wantErrIs        error
	}{
		{
			name:  "interactive, problem statement from arg",
			isTTY: true,
			opts: &CreateOptions{
				BaseRepo:         func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil },
				ProblemStatement: "task description from arg",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.CreateJobFunc = func(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "task description from arg", problemStatement)
					return &createdJobSuccessWithPR, nil
				}
			},
			wantStdout: "https://github.com/OWNER/REPO/pull/42/agent-sessions/sess1\n",
		},
		{
			name: "non-interactive, problem statement from arg",
			opts: &CreateOptions{
				BaseRepo:         func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil },
				ProblemStatement: "task description from arg",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.CreateJobFunc = func(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "task description from arg", problemStatement)
					return &createdJobSuccessWithPR, nil
				}
			},
			wantStdout: "https://github.com/OWNER/REPO/pull/42/agent-sessions/sess1\n",
		},
		{
			name:  "interactive, problem statement from file",
			isTTY: true,
			opts: &CreateOptions{
				BaseRepo:             func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil },
				ProblemStatement:     "",
				ProblemStatementFile: taskDescFile,
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.CreateJobFunc = func(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "task description from file", problemStatement)
					return &createdJobSuccessWithPR, nil
				}
			},
			wantStdout: "https://github.com/OWNER/REPO/pull/42/agent-sessions/sess1\n",
		},
		{
			name: "non-interactive, problem statement loaded from file",
			opts: &CreateOptions{
				BaseRepo:             func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil },
				ProblemStatement:     "",
				ProblemStatementFile: taskDescFile,
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.CreateJobFunc = func(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "task description from file", problemStatement)
					return &createdJobSuccessWithPR, nil
				}
			},
			wantStdout: "https://github.com/OWNER/REPO/pull/42/agent-sessions/sess1\n",
		},
		{
			name:  "interactive, problem statement from prompt/editor",
			isTTY: true,
			opts: &CreateOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				Prompter: &prompter.PrompterMock{
					MarkdownEditorFunc: func(prompt, defaultValue string, blankAllowed bool) (string, error) {
						require.Equal(t, "Enter the task description", prompt)
						return "From editor", nil
					},
				},
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.CreateJobFunc = func(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*capi.Job, error) {
					require.Equal(t, "From editor", problemStatement)
					return &createdJobSuccessWithPR, nil
				}
			},
			wantStdout: "https://github.com/OWNER/REPO/pull/42/agent-sessions/sess1\n",
		},
		{
			name:  "interactive, empty task description from editor returns error",
			isTTY: true,
			opts: &CreateOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				Prompter: &prompter.PrompterMock{
					MarkdownEditorFunc: func(prompt, defaultValue string, blankAllowed bool) (string, error) {
						return "   ", nil
					},
				},
			},
			wantErr: "a task description is required",
		},
		{
			name: "missing repo returns error",
			opts: &CreateOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return nil, nil
				}},
			wantErr: "a repository is required; re-run in a repository or supply one with --repo owner/name",
		},
		{
			name: "problem statement loaded from arg non-interactively doesn't prompt or return error",
			opts: &CreateOptions{
				BaseRepo:         func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil },
				ProblemStatement: "task description",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.CreateJobFunc = func(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "task description", problemStatement)
					return &createdJobSuccessWithPR, nil
				}
			},
			wantStdout: "https://github.com/OWNER/REPO/pull/42/agent-sessions/sess1\n",
		},
		{
			name: "base branch included in create payload",
			opts: &CreateOptions{
				BaseRepo:         func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil },
				ProblemStatement: "Do the thing",
				BaseBranch:       "feature",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.CreateJobFunc = func(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "Do the thing", problemStatement)
					require.Equal(t, "feature", baseBranch)
					return &createdJobSuccess, nil
				}
				m.GetJobFunc = func(ctx context.Context, owner, repo, jobID string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "job123", jobID)
					return &createdJobSuccessWithPR, nil
				}
			},
			wantStdout: "https://github.com/OWNER/REPO/pull/42/agent-sessions/sess1\n",
		},
		{
			name: "create task API failure returns error",
			opts: &CreateOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				ProblemStatement: "Do the thing",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.CreateJobFunc = func(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "Do the thing", problemStatement)
					require.Equal(t, "", baseBranch)
					return nil, errors.New("some API error")
				}
			},
			wantErr: "some API error",
		},
		{
			name: "get job API failure surfaces error",
			opts: &CreateOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				ProblemStatement: "Do the thing",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.CreateJobFunc = func(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "Do the thing", problemStatement)
					require.Equal(t, "", baseBranch)
					return &createdJobSuccess, nil
				}
				m.GetJobFunc = func(ctx context.Context, owner, repo, jobID string) (*capi.Job, error) {
					return nil, errors.New("some error")
				}
			},
			wantStdErr: "some error\n",
			wantStdout: "job job123 queued. View progress: https://github.com/copilot/agents\n",
		},
		{
			name: "success with immediate PR",
			opts: &CreateOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				ProblemStatement: "Do the thing",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.CreateJobFunc = func(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "Do the thing", problemStatement)
					require.Equal(t, "", baseBranch)
					return &createdJobSuccessWithPR, nil
				}
			},
			wantStdout: "https://github.com/OWNER/REPO/pull/42/agent-sessions/sess1\n",
		},
		{
			name: "success with delayed PR after polling",
			opts: &CreateOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				ProblemStatement: "Do the thing",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.CreateJobFunc = func(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "Do the thing", problemStatement)
					require.Equal(t, "", baseBranch)
					return &createdJobSuccess, nil
				}
				m.GetJobFunc = func(ctx context.Context, owner, repo, jobID string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "job123", jobID)
					return &createdJobSuccessWithPR, nil
				}
			},
			wantStdout: "https://github.com/OWNER/REPO/pull/42/agent-sessions/sess1\n",
		},
		{
			name: "fallback after polling timeout returns link to global agents page",
			opts: &CreateOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				ProblemStatement: "Do the thing",
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.CreateJobFunc = func(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "Do the thing", problemStatement)
					require.Equal(t, "", baseBranch)
					return &createdJobSuccess, nil
				}

				count := 0
				m.GetJobFunc = func(ctx context.Context, owner, repo, jobID string) (*capi.Job, error) {
					if count++; count > 4 {
						require.FailNow(t, "too many get calls")
					}
					return &createdJobSuccess, nil
				}
			},
			wantStdout: "job job123 queued. View progress: https://github.com/copilot/agents\n",
		},
		{
			name: "success with follow logs and delayed PR after polling",
			opts: &CreateOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				ProblemStatement: "Do the thing",
				Follow:           true,
				Sleep:            func(d time.Duration) {},
			},
			capiStubs: func(t *testing.T, m *capi.CapiClientMock) {
				m.CreateJobFunc = func(ctx context.Context, owner, repo, problemStatement, baseBranch, customAgent string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "Do the thing", problemStatement)
					require.Equal(t, "", baseBranch)
					return &createdJobSuccess, nil
				}
				m.GetJobFunc = func(ctx context.Context, owner, repo, jobID string) (*capi.Job, error) {
					require.Equal(t, "OWNER", owner)
					require.Equal(t, "REPO", repo)
					require.Equal(t, "job123", jobID)
					return &createdJobSuccessWithPR, nil
				}

				var count int
				m.GetSessionLogsFunc = func(_ context.Context, id string) ([]byte, error) {
					assert.Equal(t, "sess1", id)

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
			wantStdout: heredoc.Doc(`
				Displaying session logs for job job123. Press Ctrl+C to stop.
				(rendered:) <raw-logs-one>
				(rendered:) <raw-logs-two>
			`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capiClientMock := &capi.CapiClientMock{}
			if tt.capiStubs != nil {
				tt.capiStubs(t, capiClientMock)
			}

			ios, _, stdout, stderr := iostreams.Test()
			if tt.isTTY {
				ios.SetStdinTTY(true)
				ios.SetStderrTTY(true)
				ios.SetStdoutTTY(true)
			}

			tt.opts.IO = ios
			tt.opts.CapiClient = func() (capi.CapiClient, error) {
				return capiClientMock, nil
			}

			// fast backoff
			tt.opts.BackOff = backoff.WithMaxRetries(&backoff.ZeroBackOff{}, 3)

			logRenderer := &shared.LogRendererMock{}
			if tt.logRendererStubs != nil {
				tt.logRendererStubs(t, logRenderer)
			}
			tt.opts.LogRenderer = func() shared.LogRenderer {
				return logRenderer
			}

			err := createRun(tt.opts)
			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Equal(t, tt.wantErr, err.Error())
			} else if tt.wantErrIs == nil {
				require.NoError(t, err)
			}

			require.Equal(t, tt.wantStdout, stdout.String())
			require.Equal(t, tt.wantStdErr, stderr.String())
		})
	}
}
