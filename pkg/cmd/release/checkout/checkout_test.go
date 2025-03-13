package checkout

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	cliContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdCheckout(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		isTTY   bool
		want    CheckoutOptions
		wantErr string
	}{
		{
			name:  "no arguments",
			args:  "",
			isTTY: true,
			want: CheckoutOptions{
				TagName: "",
			},
		},
		{
			name:  "specific tag",
			args:  "v1.2.3",
			isTTY: true,
			want: CheckoutOptions{
				TagName: "v1.2.3",
			},
		},
		{
			name:  "force flag",
			args:  "--force v1.2.3",
			isTTY: true,
			want: CheckoutOptions{
				TagName: "v1.2.3",
				Force:   true,
			},
		},
		{
			name:  "custom branch",
			args:  "-b my-branch v1.2.3",
			isTTY: true,
			want: CheckoutOptions{
				TagName:    "v1.2.3",
				BranchName: "my-branch",
			},
		},
		{
			name:  "recurse submodules",
			args:  "--recurse-submodules v1.2.3",
			isTTY: true,
			want: CheckoutOptions{
				TagName:           "v1.2.3",
				RecurseSubmodules: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			var opts *CheckoutOptions
			cmd := NewCmdCheckout(f, func(o *CheckoutOptions) error {
				opts = o
				return nil
			})
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
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want.TagName, opts.TagName)
			assert.Equal(t, tt.want.Force, opts.Force)
			assert.Equal(t, tt.want.BranchName, opts.BranchName)
			assert.Equal(t, tt.want.RecurseSubmodules, opts.RecurseSubmodules)
		})
	}
}

func Test_checkoutRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *CheckoutOptions
		runStubs   func(*run.CommandStubber)
		stdin      string
		isTTY      bool
		wantStdout string
		wantStderr string
		wantErr    string
	}{
		{
			name: "checkout latest release",
			opts: &CheckoutOptions{
				TagName: "",
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/v1.2.3`, 1, "")
				cs.Register(`git checkout -b v1.2.3 refs/tags/v1.2.3`, 0, "")
			},
			isTTY:      true,
			wantStdout: "✓ Checked out v1.2.3 to v1.2.3\n",
			wantStderr: "",
		},
		{
			name: "checkout specific release",
			opts: &CheckoutOptions{
				TagName: "v1.2.3",
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/v1.2.3`, 1, "")
				cs.Register(`git checkout -b v1.2.3 refs/tags/v1.2.3`, 0, "")
			},
			isTTY:      true,
			wantStdout: "✓ Checked out v1.2.3 to v1.2.3\n",
			wantStderr: "",
		},
		{
			name: "branch exists, TTY, confirm yes",
			opts: &CheckoutOptions{
				TagName: "v1.2.3",
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/v1.2.3`, 0, "")
				cs.Register(`git checkout v1.2.3`, 0, "")
				cs.Register(`git merge --ff-only refs/tags/v1.2.3`, 0, "")
			},
			stdin:      "y\n",
			isTTY:      true,
			wantStdout: "A branch named 'v1.2.3' already exists. Proceeding may overwrite local changes.\nDo you want to proceed? [y/N] ✓ Checked out v1.2.3 to v1.2.3\n",
			wantStderr: "",
		},
		{
			name: "branch exists, TTY, confirm no",
			opts: &CheckoutOptions{
				TagName: "v1.2.3",
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git show-ref --verify -- refs/heads/v1.2.3`, 0, "")
			},
			stdin:      "n\n",
			isTTY:      true,
			wantStdout: "A branch named 'v1.2.3' already exists. Proceeding may overwrite local changes.\nDo you want to proceed? [y/N] ! Checkout aborted\n",
			wantStderr: "",
		},
		{
			name: "branch exists, non-TTY",
			opts: &CheckoutOptions{
				TagName: "v1.2.3",
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/v1.2.3`, 0, "")
				cs.Register(`git checkout v1.2.3`, 0, "")
				cs.Register(`git merge --ff-only refs/tags/v1.2.3`, 0, "")
			},
			isTTY:      false,
			wantStdout: "",
			wantStderr: "",
		},
		{
			name: "force flag",
			opts: &CheckoutOptions{
				TagName: "v1.2.3",
				Force:   true,
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/v1.2.3`, 0, "")
				cs.Register(`git checkout v1.2.3`, 0, "")
				cs.Register(`git reset --hard refs/tags/v1.2.3`, 0, "")
			},
			isTTY:      true,
			wantStdout: "✓ Checked out v1.2.3 to v1.2.3\n",
			wantStderr: "",
		},
		{
			name: "custom branch name",
			opts: &CheckoutOptions{
				TagName:    "v1.2.3",
				BranchName: "my-branch",
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/my-branch`, 1, "")
				cs.Register(`git checkout -b my-branch refs/tags/v1.2.3`, 0, "")
			},
			isTTY:      true,
			wantStdout: "✓ Checked out v1.2.3 to my-branch\n",
			wantStderr: "",
		},
		{
			name: "recurse submodules",
			opts: &CheckoutOptions{
				TagName:           "v1.2.3",
				RecurseSubmodules: true,
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/v1.2.3`, 1, "")
				cs.Register(`git checkout -b v1.2.3 refs/tags/v1.2.3`, 0, "")
				cs.Register(`git submodule sync --recursive`, 0, "")
				cs.Register(`git submodule update --init --recursive`, 0, "")
			},
			isTTY:      true,
			wantStdout: "✓ Checked out v1.2.3 to v1.2.3\n",
			wantStderr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			if tt.stdin != "" {
				stdin.WriteString(tt.stdin)
			}

			fakeHTTP := &httpmock.Registry{}
			defer fakeHTTP.Verify(t)

			shared.StubFetchRelease(t, fakeHTTP, "OWNER", "REPO", tt.opts.TagName, `{"tag_name": "v1.2.3"}`)

			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)
			if tt.runStubs != nil {
				tt.runStubs(cs)
			}

			tt.opts.IO = ios
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: fakeHTTP}, nil
			}
			tt.opts.GitClient = &git.Client{GhPath: "gh", GitPath: "git"}
			tt.opts.Remotes = func() (cliContext.Remotes, error) {
				repo, err := ghrepo.FromFullName("OWNER/REPO")
				if err != nil {
					t.Fatal(err)
				}
				return cliContext.Remotes{
					{Remote: &git.Remote{Name: "origin"}, Repo: repo},
				}, nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("OWNER/REPO")
			}
			tt.opts.Config = func() (gh.Config, error) {
				return config.NewBlankConfig(), nil
			}

			err := checkoutRun(tt.opts)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
