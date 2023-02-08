package base

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdSetDefault(t *testing.T) {
	tests := []struct {
		name     string
		gitStubs func(*run.CommandStubber)
		input    string
		output   SetDefaultOptions
		wantErr  bool
		errMsg   string
	}{
		{
			name: "no argument",
			gitStubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --git-dir`, 0, ".git")
			},
			input:  "",
			output: SetDefaultOptions{},
		},
		{
			name: "repo argument",
			gitStubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --git-dir`, 0, ".git")
			},
			input:  "cli/cli",
			output: SetDefaultOptions{Repo: ghrepo.New("cli", "cli")},
		},
		{
			name:     "invalid repo argument",
			gitStubs: func(cs *run.CommandStubber) {},
			input:    "some_invalid_format",
			wantErr:  true,
			errMsg:   `expected the "[HOST/]OWNER/REPO" format, got "some_invalid_format"`,
		},
		{
			name: "view flag",
			gitStubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --git-dir`, 0, ".git")
			},
			input:  "--view",
			output: SetDefaultOptions{ViewMode: true},
		},
		{
			name: "unset flag",
			gitStubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --git-dir`, 0, ".git")
			},
			input:  "--unset",
			output: SetDefaultOptions{UnsetMode: true},
		},
		{
			name: "run from non-git directory",
			gitStubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --git-dir`, 128, "")
			},
			input:   "",
			wantErr: true,
			errMsg:  "must be run from inside a git repository",
		},
	}

	for _, tt := range tests {
		io, _, _, _ := iostreams.Test()
		io.SetStdoutTTY(true)
		io.SetStdinTTY(true)
		io.SetStderrTTY(true)
		f := &cmdutil.Factory{
			IOStreams: io,
			GitClient: &git.Client{GitPath: "/fake/path/to/git"},
		}

		var gotOpts *SetDefaultOptions
		cmd := NewCmdSetDefault(f, func(opts *SetDefaultOptions) error {
			gotOpts = opts
			return nil
		})
		cmd.SetIn(&bytes.Buffer{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		t.Run(tt.name, func(t *testing.T) {
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

			cmd.SetArgs(argv)

			cs, teardown := run.Stub()
			defer teardown(t)
			tt.gitStubs(cs)

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.Repo, gotOpts.Repo)
			assert.Equal(t, tt.output.ViewMode, gotOpts.ViewMode)
		})
	}
}

func TestDefaultRun(t *testing.T) {
	repo1, _ := ghrepo.FromFullName("OWNER/REPO")
	repo2, _ := ghrepo.FromFullName("OWNER2/REPO2")
	repo3, _ := ghrepo.FromFullName("OWNER3/REPO3")
	repo4, _ := ghrepo.FromFullName("OWNER4/REPO4")
	repo5, _ := ghrepo.FromFullName("OWNER5/REPO5")
	repo6, _ := ghrepo.FromFullName("OWNER6/REPO6")

	tests := []struct {
		name          string
		tty           bool
		opts          SetDefaultOptions
		remotes       []*context.Remote
		httpStubs     func(*httpmock.Registry)
		gitStubs      func(*run.CommandStubber)
		prompterStubs func(*prompter.PrompterMock)
		wantStdout    string
		wantErr       bool
		errMsg        string
	}{
		{
			name: "unset mode with base resolved current default",
			tty:  true,
			opts: SetDefaultOptions{UnsetMode: true},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin", Resolved: "base"},
					Repo:   repo1,
				},
			},
			gitStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --unset remote.origin.gh-resolved`, 0, "")
			},
			wantStdout: "✓ Unset OWNER/REPO as default repository\n",
		},
		{
			name: "unset mode no current default",
			tty:  true,
			opts: SetDefaultOptions{UnsetMode: true},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin"},
					Repo:   repo1,
				},
			},
			wantStdout: "no default repository has been set\n",
		},
		{
			name: "view mode no current default",
			opts: SetDefaultOptions{ViewMode: true},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin"},
					Repo:   repo1,
				},
			},
			wantStdout: "no default repository has been set; use `gh repo set-default` to select one\n",
		},
		{
			name: "view mode with base resolved current default",
			opts: SetDefaultOptions{ViewMode: true},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin", Resolved: "base"},
					Repo:   repo1,
				},
			},
			wantStdout: "OWNER/REPO\n",
		},
		{
			name: "view mode with non-base resolved current default",
			opts: SetDefaultOptions{ViewMode: true},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin", Resolved: "PARENT/REPO"},
					Repo:   repo1,
				},
			},
			wantStdout: "PARENT/REPO\n",
		},
		{
			name: "tty non-interactive mode no current default",
			tty:  true,
			opts: SetDefaultOptions{Repo: repo2},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin"},
					Repo:   repo1,
				},
				{
					Remote: &git.Remote{Name: "upstream"},
					Repo:   repo2,
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryNetwork\b`),
					httpmock.StringResponse(`{"data":{"repo_000":{"name":"REPO2","owner":{"login":"OWNER2"}}}}`),
				)
			},
			gitStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --add remote.upstream.gh-resolved base`, 0, "")
			},
			wantStdout: "✓ Set OWNER2/REPO2 as the default repository for the current directory\n",
		},
		{
			name: "tty non-interactive mode set non-base default",
			tty:  true,
			opts: SetDefaultOptions{Repo: repo2},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin"},
					Repo:   repo1,
				},
				{
					Remote: &git.Remote{Name: "upstream"},
					Repo:   repo3,
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryNetwork\b`),
					httpmock.StringResponse(`{"data":{"repo_000":{"name":"REPO","owner":{"login":"OWNER"},"parent":{"name":"REPO2","owner":{"login":"OWNER2"}}}}}`),
				)
			},
			gitStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --add remote.upstream.gh-resolved OWNER2/REPO2`, 0, "")
			},
			wantStdout: "✓ Set OWNER2/REPO2 as the default repository for the current directory\n",
		},
		{
			name: "non-tty non-interactive mode no current default",
			opts: SetDefaultOptions{Repo: repo2},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin"},
					Repo:   repo1,
				},
				{
					Remote: &git.Remote{Name: "upstream"},
					Repo:   repo2,
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryNetwork\b`),
					httpmock.StringResponse(`{"data":{"repo_000":{"name":"REPO2","owner":{"login":"OWNER2"}}}}`),
				)
			},
			gitStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --add remote.upstream.gh-resolved base`, 0, "")
			},
			wantStdout: "",
		},
		{
			name: "non-interactive mode with current default",
			tty:  true,
			opts: SetDefaultOptions{Repo: repo2},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin", Resolved: "base"},
					Repo:   repo1,
				},
				{
					Remote: &git.Remote{Name: "upstream"},
					Repo:   repo2,
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryNetwork\b`),
					httpmock.StringResponse(`{"data":{"repo_000":{"name":"REPO2","owner":{"login":"OWNER2"}}}}`),
				)
			},
			gitStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --unset remote.origin.gh-resolved`, 0, "")
				cs.Register(`git config --add remote.upstream.gh-resolved base`, 0, "")
			},
			wantStdout: "✓ Set OWNER2/REPO2 as the default repository for the current directory\n",
		},
		{
			name: "non-interactive mode no known hosts",
			opts: SetDefaultOptions{Repo: repo2},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin"},
					Repo:   repo1,
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryNetwork\b`),
					httpmock.StringResponse(`{"data":{}}`),
				)
			},
			wantErr: true,
			errMsg:  "none of the git remotes correspond to a valid remote repository",
		},
		{
			name: "non-interactive mode no matching remotes",
			opts: SetDefaultOptions{Repo: repo2},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin"},
					Repo:   repo1,
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryNetwork\b`),
					httpmock.StringResponse(`{"data":{"repo_000":{"name":"REPO","owner":{"login":"OWNER"}}}}`),
				)
			},
			wantErr: true,
			errMsg:  "OWNER2/REPO2 does not correspond to any git remotes",
		},
		{
			name: "interactive mode",
			tty:  true,
			opts: SetDefaultOptions{},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin"},
					Repo:   repo1,
				},
				{
					Remote: &git.Remote{Name: "upstream"},
					Repo:   repo2,
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryNetwork\b`),
					httpmock.StringResponse(`{"data":{"repo_000":{"name":"REPO","owner":{"login":"OWNER"}},"repo_001":{"name":"REPO2","owner":{"login":"OWNER2"}}}}`),
				)
			},
			gitStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --add remote.upstream.gh-resolved base`, 0, "")
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, d string, opts []string) (int, error) {
					switch p {
					case "Which repository should be the default?":
						prompter.AssertOptions(t, []string{"OWNER/REPO", "OWNER2/REPO2"}, opts)
						return prompter.IndexFor(opts, "OWNER2/REPO2")
					default:
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			wantStdout: "This command sets the default remote repository to use when querying the\nGitHub API for the locally cloned repository.\n\ngh uses the default repository for things like:\n\n - viewing and creating pull requests\n - viewing and creating issues\n - viewing and creating releases\n - working with Actions\n - adding repository and environment secrets\n\n✓ Set OWNER2/REPO2 as the default repository for the current directory\n",
		},
		{
			name: "interactive mode only one known host",
			tty:  true,
			opts: SetDefaultOptions{},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin"},
					Repo:   repo1,
				},
				{
					Remote: &git.Remote{Name: "upstream"},
					Repo:   repo2,
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryNetwork\b`),
					httpmock.StringResponse(`{"data":{"repo_000":{"name":"REPO2","owner":{"login":"OWNER2"}}}}`),
				)
			},
			gitStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --add remote.upstream.gh-resolved base`, 0, "")
			},
			wantStdout: "Found only one known remote repo, OWNER2/REPO2 on github.com.\n✓ Set OWNER2/REPO2 as the default repository for the current directory\n",
		},
		{
			name: "interactive mode more than five remotes",
			tty:  true,
			opts: SetDefaultOptions{},
			remotes: []*context.Remote{
				{Remote: &git.Remote{Name: "origin"}, Repo: repo1},
				{Remote: &git.Remote{Name: "upstream"}, Repo: repo2},
				{Remote: &git.Remote{Name: "other1"}, Repo: repo3},
				{Remote: &git.Remote{Name: "other2"}, Repo: repo4},
				{Remote: &git.Remote{Name: "other3"}, Repo: repo5},
				{Remote: &git.Remote{Name: "other4"}, Repo: repo6},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryNetwork\b`),
					httpmock.GraphQLQuery(`{"data":{
            "repo_000":{"name":"REPO","owner":{"login":"OWNER"}},
            "repo_001":{"name":"REPO2","owner":{"login":"OWNER2"}},
            "repo_002":{"name":"REPO3","owner":{"login":"OWNER3"}},
            "repo_003":{"name":"REPO4","owner":{"login":"OWNER4"}},
            "repo_004":{"name":"REPO5","owner":{"login":"OWNER5"}},
            "repo_005":{"name":"REPO6","owner":{"login":"OWNER6"}}
          }}`,
						func(query string, inputs map[string]interface{}) {
							assert.Contains(t, query, "repo_000")
							assert.Contains(t, query, "repo_001")
							assert.Contains(t, query, "repo_002")
							assert.Contains(t, query, "repo_003")
							assert.Contains(t, query, "repo_004")
							assert.Contains(t, query, "repo_005")
						}),
				)
			},
			gitStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --add remote.upstream.gh-resolved base`, 0, "")
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, d string, opts []string) (int, error) {
					switch p {
					case "Which repository should be the default?":
						prompter.AssertOptions(t, []string{"OWNER/REPO", "OWNER2/REPO2", "OWNER3/REPO3", "OWNER4/REPO4", "OWNER5/REPO5", "OWNER6/REPO6"}, opts)
						return prompter.IndexFor(opts, "OWNER2/REPO2")
					default:
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			wantStdout: "This command sets the default remote repository to use when querying the\nGitHub API for the locally cloned repository.\n\ngh uses the default repository for things like:\n\n - viewing and creating pull requests\n - viewing and creating issues\n - viewing and creating releases\n - working with Actions\n - adding repository and environment secrets\n\n✓ Set OWNER2/REPO2 as the default repository for the current directory\n",
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

		io, _, stdout, _ := iostreams.Test()
		io.SetStdinTTY(tt.tty)
		io.SetStdoutTTY(tt.tty)
		io.SetStderrTTY(tt.tty)
		tt.opts.IO = io

		tt.opts.Remotes = func() (context.Remotes, error) {
			return tt.remotes, nil
		}

		tt.opts.GitClient = &git.Client{}

		pm := &prompter.PrompterMock{}
		if tt.prompterStubs != nil {
			tt.prompterStubs(pm)
		}

		tt.opts.Prompter = pm

		t.Run(tt.name, func(t *testing.T) {
			cs, teardown := run.Stub()
			defer teardown(t)
			if tt.gitStubs != nil {
				tt.gitStubs(cs)
			}
			defer reg.Verify(t)
			err := setDefaultRun(&tt.opts)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}
