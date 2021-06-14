package sync

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/repo/sync/syncfakes"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdSync(t *testing.T) {
	tests := []struct {
		name    string
		tty     bool
		input   string
		output  SyncOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:   "no argument",
			tty:    true,
			input:  "",
			output: SyncOptions{},
		},
		{
			name:  "destination repo",
			tty:   true,
			input: "cli/cli",
			output: SyncOptions{
				DestArg: "cli/cli",
			},
		},
		{
			name:  "source repo",
			tty:   true,
			input: "--source cli/cli",
			output: SyncOptions{
				SrcArg: "cli/cli",
			},
		},
		{
			name:  "branch",
			tty:   true,
			input: "--branch trunk",
			output: SyncOptions{
				Branch: "trunk",
			},
		},
		{
			name:  "force",
			tty:   true,
			input: "--force",
			output: SyncOptions{
				Force: true,
			},
		},
		{
			name:  "confirm",
			tty:   true,
			input: "--confirm",
			output: SyncOptions{
				SkipConfirm: true,
			},
		},
		{
			name:    "notty without confirm",
			tty:     false,
			input:   "",
			wantErr: true,
			errMsg:  "`--confirm` required when not running interactively",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			io.SetStdinTTY(tt.tty)
			io.SetStdoutTTY(tt.tty)
			f := &cmdutil.Factory{
				IOStreams: io,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *SyncOptions
			cmd := NewCmdSync(f, func(opts *SyncOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.DestArg, gotOpts.DestArg)
			assert.Equal(t, tt.output.SrcArg, gotOpts.SrcArg)
			assert.Equal(t, tt.output.Branch, gotOpts.Branch)
			assert.Equal(t, tt.output.Force, gotOpts.Force)
			assert.Equal(t, tt.output.SkipConfirm, gotOpts.SkipConfirm)
		})
	}
}

func Test_SyncRun(t *testing.T) {
	stubConfirm := func(as *prompt.AskStubber) {
		as.StubOne(true)
	}

	tests := []struct {
		name       string
		tty        bool
		opts       *SyncOptions
		remotes    []*context.Remote
		httpStubs  func(*httpmock.Registry)
		gitStubs   func(*syncfakes.FakeGitClient)
		askStubs   func(*prompt.AskStubber)
		wantStdout string
		wantStderr string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "sync local repo with parent - tty",
			tty:  true,
			opts: &SyncOptions{},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
			},
			askStubs:   stubConfirm,
			wantStdout: "✓ Synced .:trunk from OWNER/REPO:trunk\n",
		},
		{
			name: "sync local repo with parent - notty",
			tty:  false,
			opts: &SyncOptions{
				SkipConfirm: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
			},
			askStubs:   stubConfirm,
			wantStdout: "",
		},
		{
			name: "sync local repo with specified source repo",
			tty:  true,
			opts: &SyncOptions{
				SrcArg: "OWNER2/REPO2",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
			},
			askStubs:   stubConfirm,
			wantStdout: "✓ Synced .:trunk from OWNER2/REPO2:trunk\n",
		},
		{
			name: "sync local repo with parent and specified branch",
			tty:  true,
			opts: &SyncOptions{
				Branch: "test",
			},
			askStubs:   stubConfirm,
			wantStdout: "✓ Synced .:test from OWNER/REPO:test\n",
		},
		{
			name: "sync local repo with parent and force specified",
			tty:  true,
			opts: &SyncOptions{
				Force: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
			},
			gitStubs: func(fgc *syncfakes.FakeGitClient) {
				fgc.HasLocalBranchReturns(true)
				fgc.IsAncestorReturns(false, nil)
			},
			askStubs:   stubConfirm,
			wantStderr: "! Using --force will cause diverging commits on .:trunk to be discarded\n",
			wantStdout: "✓ Synced .:trunk from OWNER/REPO:trunk\n",
		},
		{
			name: "sync local repo with parent and not fast forward merge",
			tty:  true,
			opts: &SyncOptions{},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
			},
			gitStubs: func(fgc *syncfakes.FakeGitClient) {
				fgc.HasLocalBranchReturns(true)
				fgc.IsAncestorReturns(false, nil)
			},
			askStubs: stubConfirm,
			wantErr:  true,
			errMsg:   "can't sync because there are diverging commits, you can use `--force` to overwrite the commits on .:trunk",
		},
		{
			name: "sync remote fork with parent - tty",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "OWNER/REPO-FORK",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFindParent\b`),
					httpmock.StringResponse(`{"data":{"repository":{"parent":{"name":"REPO","owner":{"login": "OWNER"}}}}}`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/git/refs/heads/trunk"),
					httpmock.StringResponse(`{"object":{"sha":"0xDEADBEEF"}}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO-FORK/git/refs/heads/trunk"),
					httpmock.StringResponse(`{}`))
			},
			askStubs:   stubConfirm,
			wantStdout: "✓ Synced OWNER/REPO-FORK:trunk from OWNER/REPO:trunk\n",
		},
		{
			name: "sync remote fork with parent - notty",
			tty:  false,
			opts: &SyncOptions{
				DestArg:     "OWNER/REPO-FORK",
				SkipConfirm: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFindParent\b`),
					httpmock.StringResponse(`{"data":{"repository":{"parent":{"name":"REPO","owner":{"login": "OWNER"}}}}}`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/git/refs/heads/trunk"),
					httpmock.StringResponse(`{"object":{"sha":"0xDEADBEEF"}}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO-FORK/git/refs/heads/trunk"),
					httpmock.StringResponse(`{}`))
			},
			wantStdout: "",
		},
		{
			name: "sync remote repo with no parent",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "OWNER/REPO",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFindParent\b`),
					httpmock.StringResponse(`{"data":{"repository":{}}}`))
			},
			wantErr: true,
			errMsg:  "can't determine source repo for OWNER/REPO because repo is not fork",
		},
		{
			name: "sync remote repo with specified source repo",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "OWNER/REPO",
				SrcArg:  "OWNER2/REPO2",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER2/REPO2/git/refs/heads/trunk"),
					httpmock.StringResponse(`{"object":{"sha":"0xDEADBEEF"}}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/git/refs/heads/trunk"),
					httpmock.StringResponse(`{}`))
			},
			askStubs:   stubConfirm,
			wantStdout: "✓ Synced OWNER/REPO:trunk from OWNER2/REPO2:trunk\n",
		},
		{
			name: "sync remote fork with parent and specified branch",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "OWNER/REPO-FORK",
				Branch:  "test",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFindParent\b`),
					httpmock.StringResponse(`{"data":{"repository":{"parent":{"name":"REPO","owner":{"login": "OWNER"}}}}}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/git/refs/heads/test"),
					httpmock.StringResponse(`{"object":{"sha":"0xDEADBEEF"}}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO-FORK/git/refs/heads/test"),
					httpmock.StringResponse(`{}`))
			},
			askStubs:   stubConfirm,
			wantStdout: "✓ Synced OWNER/REPO-FORK:test from OWNER/REPO:test\n",
		},
		{
			name: "sync remote fork with parent and force specified",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "OWNER/REPO-FORK",
				Force:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFindParent\b`),
					httpmock.StringResponse(`{"data":{"repository":{"parent":{"name":"REPO","owner":{"login": "OWNER"}}}}}`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/git/refs/heads/trunk"),
					httpmock.StringResponse(`{"object":{"sha":"0xDEADBEEF"}}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO-FORK/git/refs/heads/trunk"),
					httpmock.StringResponse(`{}`))
			},
			askStubs:   stubConfirm,
			wantStderr: "! Using --force will cause diverging commits on OWNER/REPO-FORK:trunk to be discarded\n",
			wantStdout: "✓ Synced OWNER/REPO-FORK:trunk from OWNER/REPO:trunk\n",
		},
		{
			name: "sync remote fork with parent and not fast forward merge",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "OWNER/REPO-FORK",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFindParent\b`),
					httpmock.StringResponse(`{"data":{"repository":{"parent":{"name":"REPO","owner":{"login": "OWNER"}}}}}`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/git/refs/heads/trunk"),
					httpmock.StringResponse(`{"object":{"sha":"0xDEADBEEF"}}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO-FORK/git/refs/heads/trunk"),
					func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: 422,
							Request:    req,
							Header:     map[string][]string{"Content-Type": {"application/json"}},
							Body:       ioutil.NopCloser(bytes.NewBufferString(`{"message":"Update is not a fast forward"}`)),
						}, nil
					})
			},
			askStubs: stubConfirm,
			wantErr:  true,
			errMsg:   "can't sync because there are diverging commits, you can use `--force` to overwrite the commits on OWNER/REPO-FORK:trunk",
		},
	}
	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		io, _, stdout, stderr := iostreams.Test()
		io.SetStdinTTY(tt.tty)
		io.SetStdoutTTY(tt.tty)
		tt.opts.IO = io

		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			repo, _ := ghrepo.FromFullName("OWNER/REPO")
			return repo, nil
		}

		tt.opts.Remotes = func() (context.Remotes, error) {
			if tt.remotes == nil {
				return []*context.Remote{{Remote: &git.Remote{Name: "origin"}}}, nil
			}
			return tt.remotes, nil
		}

		var gitClient = &syncfakes.FakeGitClient{}
		if tt.gitStubs != nil {
			tt.gitStubs(gitClient)
		}
		tt.opts.Git = gitClient

		as, teardown := prompt.InitAskStubber()
		defer teardown()
		if tt.askStubs != nil {
			tt.askStubs(as)
		}

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			err := syncRun(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}
