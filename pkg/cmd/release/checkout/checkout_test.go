package checkout

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
			name:    "no arguments",
			args:    "",
			isTTY:   true,
			wantErr: "not in a Git repository and no --repo specified.\nPlease run from a Git repository or use --repo to specify a repository to checkout release",
		},
		{
			name:  "specific release",
			args:  "v1.2.3",
			isTTY: true,
			want: CheckoutOptions{
				TagName: "v1.2.3",
			},
			wantErr: "not in a Git repository and no --repo specified.\nPlease run from a Git repository or use --repo to specify a repository to checkout release",
		},
		{
			name:  "force",
			args:  "--force v1.2.3",
			isTTY: true,
			want: CheckoutOptions{
				TagName: "v1.2.3",
				Force:   true,
			},
			wantErr: "not in a Git repository and no --repo specified.\nPlease run from a Git repository or use --repo to specify a repository to checkout release",
		},
		{
			name:  "custom branch",
			args:  "-b my-branch v1.2.3",
			isTTY: true,
			want: CheckoutOptions{
				TagName:    "v1.2.3",
				BranchName: "my-branch",
			},
			wantErr: "not in a Git repository and no --repo specified.\nPlease run from a Git repository or use --repo to specify a repository to checkout release",
		},
		{
			name:  "recurse submodules",
			args:  "--recurse-submodules v1.2.3",
			isTTY: true,
			want: CheckoutOptions{
				TagName:           "v1.2.3",
				RecurseSubmodules: true,
			},
			wantErr: "not in a Git repository and no --repo specified.\nPlease run from a Git repository or use --repo to specify a repository to checkout release",
		},
		{
			name:    "no arguments, non-TTY",
			args:    "",
			isTTY:   false,
			wantErr: "release tag argument required when not running interactively",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			f := &cmdutil.Factory{
				IOStreams: ios,
				Remotes: func() (cliContext.Remotes, error) {
					return nil, fmt.Errorf("not a git repo")
				},
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
			require.NotNil(t, opts, "opts should be initialized")
			assert.Equal(t, tt.want.TagName, opts.TagName)
			assert.Equal(t, tt.want.Force, opts.Force)
			assert.Equal(t, tt.want.BranchName, opts.BranchName)
			assert.Equal(t, tt.want.RecurseSubmodules, opts.RecurseSubmodules)
			assert.False(t, opts.IsLocalGitRepo)
		})
	}
}

func Test_checkoutRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *CheckoutOptions
		runStubs   func(*run.CommandStubber)
		httpStubs  func(*httpmock.Registry)
		stdin      string
		isTTY      bool
		wantStdout string
		wantStderr string
		wantErr    string
	}{
		{
			name: "checkout latest release in local repo",
			opts: &CheckoutOptions{
				TagName:        "",
				IsLocalGitRepo: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				shared.StubFetchRelease(t, reg, "OWNER", "REPO", "", `{"tag_name": "v1.2.3"}`)
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/v1.2.3`, 1, "")
				cs.Register(`git checkout -b v1.2.3 refs/tags/v1.2.3`, 0, "")
			},
			isTTY:      true,
			wantStdout: "✓ Checked out v1.2.3 to v1.2.3\n",
		},
		{
			name: "checkout specific release in local repo",
			opts: &CheckoutOptions{
				TagName:        "v1.2.3",
				IsLocalGitRepo: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				shared.StubFetchRelease(t, reg, "OWNER", "REPO", "v1.2.3", `{"tag_name": "v1.2.3"}`)
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/v1.2.3`, 1, "")
				cs.Register(`git checkout -b v1.2.3 refs/tags/v1.2.3`, 0, "")
			},
			isTTY:      true,
			wantStdout: "✓ Checked out v1.2.3 to v1.2.3\n",
		},
		{
			name: "branch exists, TTY, confirm yes",
			opts: &CheckoutOptions{
				TagName:        "v1.2.3",
				IsLocalGitRepo: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				shared.StubFetchRelease(t, reg, "OWNER", "REPO", "v1.2.3", `{"tag_name": "v1.2.3"}`)
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/v1.2.3`, 0, "")
				cs.Register(`git checkout v1.2.3`, 0, "")
				cs.Register(`git merge --ff-only refs/tags/v1.2.3`, 0, "")
			},
			stdin:      "y\n",
			isTTY:      true,
			wantStdout: "Branch 'v1.2.3' already exists. Overwrite with v1.2.3? [y/N] ✓ Checked out v1.2.3 to v1.2.3\n",
		},
		{
			name: "branch exists, TTY, confirm no",
			opts: &CheckoutOptions{
				TagName:        "v1.2.3",
				IsLocalGitRepo: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				shared.StubFetchRelease(t, reg, "OWNER", "REPO", "v1.2.3", `{"tag_name": "v1.2.3"}`)
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/v1.2.3`, 0, "")
			},
			stdin:      "n\n",
			isTTY:      true,
			wantStdout: "Branch 'v1.2.3' already exists. Overwrite with v1.2.3? [y/N] ! Checkout aborted\n",
			wantErr:    "SilentError",
		},
		{
			name: "branch exists, non-TTY",
			opts: &CheckoutOptions{
				TagName:        "v1.2.3",
				IsLocalGitRepo: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				shared.StubFetchRelease(t, reg, "OWNER", "REPO", "v1.2.3", `{"tag_name": "v1.2.3"}`)
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/v1.2.3`, 0, "")
				cs.Register(`git checkout v1.2.3`, 0, "")
				cs.Register(`git merge --ff-only refs/tags/v1.2.3`, 0, "")
			},
			isTTY:      false,
			wantStdout: "",
		},
		{
			name: "force flag",
			opts: &CheckoutOptions{
				TagName:        "v1.2.3",
				Force:          true,
				IsLocalGitRepo: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				shared.StubFetchRelease(t, reg, "OWNER", "REPO", "v1.2.3", `{"tag_name": "v1.2.3"}`)
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/v1.2.3`, 0, "")
				cs.Register(`git checkout v1.2.3`, 0, "")
				cs.Register(`git reset --hard refs/tags/v1.2.3`, 0, "")
			},
			isTTY:      true,
			wantStdout: "✓ Checked out v1.2.3 to v1.2.3\n",
		},
		{
			name: "custom branch name",
			opts: &CheckoutOptions{
				TagName:        "v1.2.3",
				BranchName:     "my-branch",
				IsLocalGitRepo: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				shared.StubFetchRelease(t, reg, "OWNER", "REPO", "v1.2.3", `{"tag_name": "v1.2.3"}`)
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/tags/v1.2.3 --no-tags`, 0, "")
				cs.Register(`git show-ref --verify -- refs/heads/my-branch`, 1, "")
				cs.Register(`git checkout -b my-branch refs/tags/v1.2.3`, 0, "")
			},
			isTTY:      true,
			wantStdout: "✓ Checked out v1.2.3 to my-branch\n",
		},
		{
			name: "recurse submodules",
			opts: &CheckoutOptions{
				TagName:           "v1.2.3",
				RecurseSubmodules: true,
				IsLocalGitRepo:    true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				shared.StubFetchRelease(t, reg, "OWNER", "REPO", "v1.2.3", `{"tag_name": "v1.2.3"}`)
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
		},
		{
			name: "clone when not in git repo",
			opts: &CheckoutOptions{
				TagName:        "v1.2.3",
				IsLocalGitRepo: false,
			},
			httpStubs: func(reg *httpmock.Registry) {
				shared.StubFetchRelease(t, reg, "OWNER", "REPO", "v1.2.3", `{"tag_name": "v1.2.3"}`)
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git clone --branch v1.2.3 https://github.com/OWNER/REPO.git`, 0, "")
				cs.Register(`git checkout -b v1.2.3`, 0, "")
			},
			isTTY:      true,
			wantStdout: "✓ Cloned OWNER/REPO@v1.2.3 to REPO-v1.2.3\n",
		},
		{
			name: "clone with mismatched repo",
			opts: &CheckoutOptions{
				TagName:        "v1.2.3",
				IsLocalGitRepo: true,
			},
			isTTY:      true,
			wantStdout: "",
			wantErr:    "--repo OWNER/REPO doesn't match the current repository (OTHER/OTHER-REPO).\nTry running out of a Git repo to checkout release, or omit --repo to checkout for current repo",
		},
		{
			name: "clone with existing dir, confirm yes",
			opts: &CheckoutOptions{
				TagName:        "v1.2.3",
				IsLocalGitRepo: false,
			},
			httpStubs: func(reg *httpmock.Registry) {
				shared.StubFetchRelease(t, reg, "OWNER", "REPO", "v1.2.3", `{"tag_name": "v1.2.3"}`)
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git clone --branch v1.2.3 https://github.com/OWNER/REPO.git`, 0, "")
				cs.Register(`git checkout -b v1.2.3`, 0, "")
			},
			stdin:      "y\n",
			isTTY:      true,
			wantStdout: "Directory 'REPO-v1.2.3' already exists. Overwrite by cloning OWNER/REPO@v1.2.3? [y/N] ✓ Cloned OWNER/REPO@v1.2.3 to REPO-v1.2.3\n",
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
			if tt.httpStubs != nil {
				tt.httpStubs(fakeHTTP)
			}

			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)
			if tt.runStubs != nil {
				tt.runStubs(cs)
			}

			tempDir := t.TempDir()
			if tt.name == "clone when not in git repo" || tt.name == "clone with existing dir, confirm yes" {
				err := os.Mkdir(filepath.Join(tempDir, "REPO"), 0755)
				if err != nil {
					t.Fatalf("failed to create REPO dir: %v", err)
				}
				if tt.name == "clone with existing dir, confirm yes" {
					err = os.Mkdir(filepath.Join(tempDir, "REPO-v1.2.3"), 0755)
					if err != nil {
						t.Fatalf("failed to create REPO-v1.2.3 dir: %v", err)
					}
				}
			}

			origDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get current dir: %v", err)
			}
			err = os.Chdir(tempDir)
			if err != nil {
				t.Fatalf("failed to change to temp dir: %v", err)
			}
			defer os.Chdir(origDir)

			tt.opts.IO = ios
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: fakeHTTP}, nil
			}
			tt.opts.GitClient = &git.Client{GhPath: "gh", GitPath: "git"}
			tt.opts.Config = func() (gh.Config, error) {
				return config.NewBlankConfig(), nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("OWNER/REPO")
			}

			if tt.name == "clone with mismatched repo" {
				tt.opts.Remotes = func() (cliContext.Remotes, error) {
					repo, _ := ghrepo.FromFullName("OTHER/OTHER-REPO")
					return cliContext.Remotes{{Remote: &git.Remote{Name: "origin"}, Repo: repo}}, nil
				}
			} else if tt.opts.IsLocalGitRepo {
				tt.opts.Remotes = func() (cliContext.Remotes, error) {
					repo, _ := ghrepo.FromFullName("OWNER/REPO")
					return cliContext.Remotes{{Remote: &git.Remote{Name: "origin"}, Repo: repo}}, nil
				}
			} else {
				tt.opts.Remotes = func() (cliContext.Remotes, error) {
					return nil, fmt.Errorf("not a git repo")
				}
			}

			err = checkoutRun(tt.opts)
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
