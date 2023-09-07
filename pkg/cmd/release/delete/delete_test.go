package delete

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdDelete(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		isTTY   bool
		want    DeleteOptions
		wantErr string
	}{
		{
			name:  "version argument",
			args:  "v1.2.3",
			isTTY: true,
			want: DeleteOptions{
				TagName:     "v1.2.3",
				SkipConfirm: false,
				CleanupTag:  false,
			},
		},
		{
			name:  "skip confirm",
			args:  "v1.2.3 -y",
			isTTY: true,
			want: DeleteOptions{
				TagName:     "v1.2.3",
				SkipConfirm: true,
				CleanupTag:  false,
			},
		},
		{
			name:  "cleanup tag",
			args:  "v1.2.3 --cleanup-tag",
			isTTY: true,
			want: DeleteOptions{
				TagName:     "v1.2.3",
				SkipConfirm: false,
				CleanupTag:  true,
			},
		},
		{
			name:    "no arguments",
			args:    "",
			isTTY:   true,
			wantErr: "accepts 1 arg(s), received 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			var opts *DeleteOptions
			cmd := NewCmdDelete(f, func(o *DeleteOptions) error {
				opts = o
				return nil
			})
			cmd.PersistentFlags().StringP("repo", "R", "", "")

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)

			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want.TagName, opts.TagName)
			assert.Equal(t, tt.want.SkipConfirm, opts.SkipConfirm)
			assert.Equal(t, tt.want.CleanupTag, opts.CleanupTag)
		})
	}
}

func Test_deleteRun(t *testing.T) {
	tests := []struct {
		name          string
		isTTY         bool
		opts          DeleteOptions
		prompterStubs func(*prompter.PrompterMock)
		runStubs      func(*run.CommandStubber)
		wantErr       string
		wantStdout    string
		wantStderr    string
	}{
		{
			name:  "interactive confirm",
			isTTY: true,
			opts: DeleteOptions{
				TagName: "v1.2.3",
			},
			wantStdout: "",
			wantStderr: "✓ Deleted release v1.2.3\n! Note that the v1.2.3 git tag still remains in the repository\n",
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.ConfirmFunc = func(p string, d bool) (bool, error) {
					if p == "Delete release v1.2.3 in OWNER/REPO?" {
						return true, nil
					}
					return false, prompter.NoSuchPromptErr(p)
				}
			},
		},
		{
			name:  "skipping confirmation",
			isTTY: true,
			opts: DeleteOptions{
				TagName:     "v1.2.3",
				SkipConfirm: true,
				CleanupTag:  false,
			},
			wantStdout: ``,
			wantStderr: heredoc.Doc(`
				✓ Deleted release v1.2.3
				! Note that the v1.2.3 git tag still remains in the repository
			`),
		},
		{
			name:  "non-interactive",
			isTTY: false,
			opts: DeleteOptions{
				TagName:     "v1.2.3",
				SkipConfirm: false,
				CleanupTag:  false,
			},
			wantStdout: ``,
			wantStderr: ``,
		},
		{
			name:  "cleanup-tag & skipping confirmation",
			isTTY: true,
			opts: DeleteOptions{
				TagName:     "v1.2.3",
				SkipConfirm: true,
				CleanupTag:  true,
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git tag -d v1.2.3`, 0, "")
			},
			wantStdout: ``,
			wantStderr: heredoc.Doc(`
				✓ Deleted release and tag v1.2.3
			`),
		},
		{
			name:  "cleanup-tag",
			isTTY: false,
			opts: DeleteOptions{
				TagName:     "v1.2.3",
				SkipConfirm: false,
				CleanupTag:  true,
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git tag -d v1.2.3`, 0, "")
			},
			wantStdout: ``,
			wantStderr: ``,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			pm := &prompter.PrompterMock{}
			if tt.prompterStubs != nil {
				tt.prompterStubs(pm)
			}

			fakeHTTP := &httpmock.Registry{}
			shared.StubFetchRelease(t, fakeHTTP, "OWNER", "REPO", tt.opts.TagName, `{
				"tag_name": "v1.2.3",
				"draft": false,
				"url": "https://api.github.com/repos/OWNER/REPO/releases/23456"
			}`)

			fakeHTTP.Register(httpmock.REST("DELETE", "repos/OWNER/REPO/releases/23456"), httpmock.StatusStringResponse(204, ""))
			fakeHTTP.Register(httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/tags/v1.2.3"), httpmock.StatusStringResponse(204, ""))

			rs, teardown := run.Stub()
			defer teardown(t)
			if tt.runStubs != nil {
				tt.runStubs(rs)
			}

			tt.opts.IO = ios
			tt.opts.Prompter = pm
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: fakeHTTP}, nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("OWNER/REPO")
			}
			tt.opts.GitClient = &git.Client{GitPath: "some/path/git"}

			err := deleteRun(&tt.opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
