package fork

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdFork(t *testing.T) {
	tests := []struct {
		name    string
		cli     string
		tty     bool
		wants   ForkOptions
		wantErr bool
		errMsg  string
	}{
		{
			name: "repo with git args",
			cli:  "foo/bar -- --foo=bar",
			wants: ForkOptions{
				Repository: "foo/bar",
				GitArgs:    []string{"--foo=bar"},
				RemoteName: "origin",
				Rename:     true,
			},
		},
		{
			name:    "git args without repo",
			cli:     "-- --foo bar",
			wantErr: true,
			errMsg:  "repository argument required when passing git clone flags",
		},
		{
			name: "repo",
			cli:  "foo/bar",
			wants: ForkOptions{
				Repository: "foo/bar",
				RemoteName: "origin",
				Rename:     true,
				GitArgs:    []string{},
			},
		},
		{
			name:    "blank remote name",
			cli:     "--remote --remote-name=''",
			wantErr: true,
			errMsg:  "--remote-name cannot be blank",
		},
		{
			name: "remote name",
			cli:  "--remote --remote-name=foo",
			wants: ForkOptions{
				RemoteName: "foo",
				Rename:     false,
				Remote:     true,
			},
		},
		{
			name: "blank nontty",
			cli:  "",
			wants: ForkOptions{
				RemoteName:   "origin",
				Rename:       true,
				Organization: "",
			},
		},
		{
			name: "blank tty",
			cli:  "",
			tty:  true,
			wants: ForkOptions{
				RemoteName:   "origin",
				PromptClone:  true,
				PromptRemote: true,
				Rename:       true,
				Organization: "",
			},
		},
		{
			name: "clone",
			cli:  "--clone",
			wants: ForkOptions{
				RemoteName: "origin",
				Rename:     true,
			},
		},
		{
			name: "remote",
			cli:  "--remote",
			wants: ForkOptions{
				RemoteName: "origin",
				Remote:     true,
				Rename:     true,
			},
		},
		{
			name: "to org",
			cli:  "--org batmanshome",
			wants: ForkOptions{
				RemoteName:   "origin",
				Remote:       false,
				Rename:       false,
				Organization: "batmanshome",
			},
		},
		{
			name:    "empty org",
			cli:     " --org=''",
			wantErr: true,
			errMsg:  "--org cannot be blank",
		},
		{
			name:    "git flags in wrong place",
			cli:     "--depth 1 OWNER/REPO",
			wantErr: true,
			errMsg:  "unknown flag: --depth\nSeparate git clone flags with `--`.",
		},
		{
			name: "with fork name",
			cli:  "--fork-name new-fork",
			wants: ForkOptions{
				Remote:     false,
				RemoteName: "origin",
				ForkName:   "new-fork",
				Rename:     false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()

			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			ios.SetStdoutTTY(tt.tty)
			ios.SetStdinTTY(tt.tty)

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *ForkOptions
			cmd := NewCmdFork(f, func(opts *ForkOptions) error {
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

			assert.Equal(t, tt.wants.RemoteName, gotOpts.RemoteName)
			assert.Equal(t, tt.wants.Remote, gotOpts.Remote)
			assert.Equal(t, tt.wants.PromptRemote, gotOpts.PromptRemote)
			assert.Equal(t, tt.wants.PromptClone, gotOpts.PromptClone)
			assert.Equal(t, tt.wants.Organization, gotOpts.Organization)
			assert.Equal(t, tt.wants.GitArgs, gotOpts.GitArgs)
		})
	}
}

func TestRepoFork(t *testing.T) {
	forkResult := `{
		"node_id": "123",
		"name": "REPO",
		"clone_url": "https://github.com/someone/repo.git",
		"created_at": "2011-01-26T19:01:12Z",
		"owner": {
			"login": "someone"
		}
	}`

	forkPost := func(reg *httpmock.Registry) {
		reg.Register(
			httpmock.REST("POST", "repos/OWNER/REPO/forks"),
			httpmock.StringResponse(forkResult))
	}

	tests := []struct {
		name        string
		opts        *ForkOptions
		tty         bool
		httpStubs   func(*httpmock.Registry)
		execStubs   func(*run.CommandStubber)
		promptStubs func(*prompter.MockPrompter)
		cfgStubs    func(*testing.T, gh.Config)
		remotes     []*context.Remote
		wantOut     string
		wantErrOut  string
		wantErr     bool
		errMsg      string
	}{
		{
			name: "implicit match, configured protocol overrides provided",
			tty:  true,
			opts: &ForkOptions{
				Remote:     true,
				RemoteName: "fork",
			},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin", PushURL: &url.URL{
						Scheme: "ssh",
					}},
					Repo: ghrepo.New("OWNER", "REPO"),
				},
			},
			cfgStubs: func(_ *testing.T, c gh.Config) {
				c.Set("", "git_protocol", "https")
			},
			httpStubs: forkPost,
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git remote add fork https://github\.com/someone/REPO\.git`, 0, "")
			},
			wantErrOut: "✓ Created fork someone/REPO\n✓ Added remote fork\n",
		},
		{
			name: "implicit match, no configured protocol",
			tty:  true,
			opts: &ForkOptions{
				Remote:     true,
				RemoteName: "fork",
			},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin", PushURL: &url.URL{
						Scheme: "ssh",
					}},
					Repo: ghrepo.New("OWNER", "REPO"),
				},
			},
			httpStubs: forkPost,
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git remote add fork git@github\.com:someone/REPO\.git`, 0, "")
			},
			wantErrOut: "✓ Created fork someone/REPO\n✓ Added remote fork\n",
		},
		{
			name: "implicit with negative interactive choices",
			tty:  true,
			opts: &ForkOptions{
				PromptRemote: true,
				Rename:       true,
				RemoteName:   defaultRemoteName,
			},
			httpStubs: forkPost,
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterConfirm("Would you like to add a remote for the fork?", func(_ string, _ bool) (bool, error) {
					return false, nil
				})
			},
			wantErrOut: "✓ Created fork someone/REPO\n",
		},
		{
			name: "implicit with interactive choices",
			tty:  true,
			opts: &ForkOptions{
				PromptRemote: true,
				Rename:       true,
				RemoteName:   defaultRemoteName,
			},
			httpStubs: forkPost,
			execStubs: func(cs *run.CommandStubber) {
				cs.Register("git remote rename origin upstream", 0, "")
				cs.Register(`git remote add origin https://github.com/someone/REPO.git`, 0, "")
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterConfirm("Would you like to add a remote for the fork?", func(_ string, _ bool) (bool, error) {
					return true, nil
				})
			},
			wantErrOut: "✓ Created fork someone/REPO\n✓ Renamed remote origin to upstream\n✓ Added remote origin\n",
		},
		{
			name: "implicit tty reuse existing remote",
			tty:  true,
			opts: &ForkOptions{
				Remote:     true,
				RemoteName: defaultRemoteName,
			},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin", FetchURL: &url.URL{}},
					Repo:   ghrepo.New("someone", "REPO"),
				},
				{
					Remote: &git.Remote{Name: "upstream", FetchURL: &url.URL{}},
					Repo:   ghrepo.New("OWNER", "REPO"),
				},
			},
			httpStubs:  forkPost,
			wantErrOut: "✓ Created fork someone/REPO\n✓ Using existing remote origin\n",
		},
		{
			name: "implicit tty remote exists",
			// gh repo fork --remote --remote-name origin | cat
			tty: true,
			opts: &ForkOptions{
				Remote:     true,
				RemoteName: defaultRemoteName,
			},
			httpStubs: forkPost,
			wantErr:   true,
			errMsg:    "a git remote named 'origin' already exists",
		},
		{
			name: "implicit tty current owner forked",
			tty:  true,
			opts: &ForkOptions{
				Repository: "someone/REPO",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/someone/REPO/forks"),
					httpmock.StringResponse(forkResult))
			},
			wantErr: true,
			errMsg:  "failed to fork: someone/REPO cannot be forked",
		},
		{
			name: "implicit tty already forked",
			tty:  true,
			opts: &ForkOptions{
				Since: func(t time.Time) time.Duration {
					return 120 * time.Second
				},
			},
			httpStubs:  forkPost,
			wantErrOut: "! someone/REPO already exists\n",
		},
		{
			name: "implicit tty --remote",
			tty:  true,
			opts: &ForkOptions{
				Remote:     true,
				RemoteName: defaultRemoteName,
				Rename:     true,
			},
			httpStubs: forkPost,
			execStubs: func(cs *run.CommandStubber) {
				cs.Register("git remote rename origin upstream", 0, "")
				cs.Register(`git remote add origin https://github.com/someone/REPO.git`, 0, "")
			},
			wantErrOut: "✓ Created fork someone/REPO\n✓ Renamed remote origin to upstream\n✓ Added remote origin\n",
		},
		{
			name: "implicit nontty reuse existing remote",
			opts: &ForkOptions{
				Remote:     true,
				RemoteName: defaultRemoteName,
				Rename:     true,
			},
			remotes: []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin", FetchURL: &url.URL{}},
					Repo:   ghrepo.New("someone", "REPO"),
				},
				{
					Remote: &git.Remote{Name: "upstream", FetchURL: &url.URL{}},
					Repo:   ghrepo.New("OWNER", "REPO"),
				},
			},
			httpStubs: forkPost,
			wantOut:   "https://github.com/someone/REPO\n",
		},
		{
			name: "implicit nontty remote exists",
			// gh repo fork --remote --remote-name origin | cat
			opts: &ForkOptions{
				Remote:     true,
				RemoteName: defaultRemoteName,
			},
			httpStubs: forkPost,
			wantErr:   true,
			errMsg:    "a git remote named 'origin' already exists",
		},
		{
			name: "implicit nontty already forked",
			opts: &ForkOptions{
				Since: func(t time.Time) time.Duration {
					return 120 * time.Second
				},
			},
			httpStubs:  forkPost,
			wantErrOut: "someone/REPO already exists\n",
		},
		{
			name: "implicit nontty --remote",
			opts: &ForkOptions{
				Remote:     true,
				RemoteName: defaultRemoteName,
				Rename:     true,
			},
			httpStubs: forkPost,
			execStubs: func(cs *run.CommandStubber) {
				cs.Register("git remote rename origin upstream", 0, "")
				cs.Register(`git remote add origin https://github.com/someone/REPO.git`, 0, "")
			},
			wantOut: "https://github.com/someone/REPO\n",
		},
		{
			name:      "implicit nontty no args",
			opts:      &ForkOptions{},
			httpStubs: forkPost,
			wantOut:   "https://github.com/someone/REPO\n",
		},
		{
			name: "passes git flags",
			tty:  true,
			opts: &ForkOptions{
				Repository: "OWNER/REPO",
				GitArgs:    []string{"--depth", "1"},
				Clone:      true,
			},
			httpStubs: forkPost,
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git clone --depth 1 https://github.com/someone/REPO\.git`, 0, "")
				cs.Register(`git -C REPO remote add upstream https://github\.com/OWNER/REPO\.git`, 0, "")
				cs.Register(`git -C REPO fetch upstream`, 0, "")
				cs.Register(`git -C REPO config --add remote.upstream.gh-resolved base`, 0, "")
			},
			wantErrOut: "✓ Created fork someone/REPO\n✓ Cloned fork\n! Repository OWNER/REPO set as the default repository. To learn more about the default repository, run: gh repo set-default --help\n",
		},
		{
			name: "repo arg fork to org",
			tty:  true,
			opts: &ForkOptions{
				Repository:   "OWNER/REPO",
				Organization: "gamehendge",
				Clone:        true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/forks"),
					func(req *http.Request) (*http.Response, error) {
						bb, err := io.ReadAll(req.Body)
						if err != nil {
							return nil, err
						}
						assert.Equal(t, `{"organization":"gamehendge"}`, strings.TrimSpace(string(bb)))
						return &http.Response{
							Request:    req,
							StatusCode: 200,
							Body:       io.NopCloser(bytes.NewBufferString(`{"name":"REPO", "owner":{"login":"gamehendge"}}`)),
						}, nil
					})
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git clone https://github.com/gamehendge/REPO\.git`, 0, "")
				cs.Register(`git -C REPO remote add upstream https://github\.com/OWNER/REPO\.git`, 0, "")
				cs.Register(`git -C REPO fetch upstream`, 0, "")
				cs.Register(`git -C REPO config --add remote.upstream.gh-resolved base`, 0, "")
			},
			wantErrOut: "✓ Created fork gamehendge/REPO\n✓ Cloned fork\n! Repository OWNER/REPO set as the default repository. To learn more about the default repository, run: gh repo set-default --help\n",
		},
		{
			name: "repo arg url arg",
			tty:  true,
			opts: &ForkOptions{
				Repository: "https://github.com/OWNER/REPO.git",
				Clone:      true,
			},
			httpStubs: forkPost,
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git clone https://github.com/someone/REPO\.git`, 0, "")
				cs.Register(`git -C REPO remote add upstream https://github\.com/OWNER/REPO\.git`, 0, "")
				cs.Register(`git -C REPO fetch upstream`, 0, "")
				cs.Register(`git -C REPO config --add remote.upstream.gh-resolved base`, 0, "")
			},
			wantErrOut: "✓ Created fork someone/REPO\n✓ Cloned fork\n! Repository OWNER/REPO set as the default repository. To learn more about the default repository, run: gh repo set-default --help\n",
		},
		{
			name: "repo arg interactive no clone",
			tty:  true,
			opts: &ForkOptions{
				Repository:  "OWNER/REPO",
				PromptClone: true,
			},
			httpStubs: forkPost,
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterConfirm("Would you like to clone the fork?", func(_ string, _ bool) (bool, error) {
					return false, nil
				})
			},
			wantErrOut: "✓ Created fork someone/REPO\n",
		},
		{
			name: "repo arg interactive",
			tty:  true,
			opts: &ForkOptions{
				Repository:  "OWNER/REPO",
				PromptClone: true,
			},
			httpStubs: forkPost,
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterConfirm("Would you like to clone the fork?", func(_ string, _ bool) (bool, error) {
					return true, nil
				})
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git clone https://github.com/someone/REPO\.git`, 0, "")
				cs.Register(`git -C REPO remote add upstream https://github\.com/OWNER/REPO\.git`, 0, "")
				cs.Register(`git -C REPO fetch upstream`, 0, "")
				cs.Register(`git -C REPO config --add remote.upstream.gh-resolved base`, 0, "")
			},
			wantErrOut: "✓ Created fork someone/REPO\n✓ Cloned fork\n! Repository OWNER/REPO set as the default repository. To learn more about the default repository, run: gh repo set-default --help\n",
		},
		{
			name: "repo arg interactive already forked",
			tty:  true,
			opts: &ForkOptions{
				Repository:  "OWNER/REPO",
				PromptClone: true,
				Since: func(t time.Time) time.Duration {
					return 120 * time.Second
				},
			},
			httpStubs: forkPost,
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterConfirm("Would you like to clone the fork?", func(_ string, _ bool) (bool, error) {
					return true, nil
				})
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git clone https://github.com/someone/REPO\.git`, 0, "")
				cs.Register(`git -C REPO remote add upstream https://github\.com/OWNER/REPO\.git`, 0, "")
				cs.Register(`git -C REPO fetch upstream`, 0, "")
				cs.Register(`git -C REPO config --add remote.upstream.gh-resolved base`, 0, "")
			},
			wantErrOut: "! someone/REPO already exists\n✓ Cloned fork\n! Repository OWNER/REPO set as the default repository. To learn more about the default repository, run: gh repo set-default --help\n",
		},
		{
			name: "repo arg nontty no flags",
			opts: &ForkOptions{
				Repository: "OWNER/REPO",
			},
			httpStubs: forkPost,
			wantOut:   "https://github.com/someone/REPO\n",
		},
		{
			name: "repo arg nontty repo already exists",
			opts: &ForkOptions{
				Repository: "OWNER/REPO",
				Since: func(t time.Time) time.Duration {
					return 120 * time.Second
				},
			},
			httpStubs:  forkPost,
			wantErrOut: "someone/REPO already exists\n",
		},
		{
			name: "repo arg nontty clone arg already exists",
			opts: &ForkOptions{
				Repository: "OWNER/REPO",
				Clone:      true,
				Since: func(t time.Time) time.Duration {
					return 120 * time.Second
				},
			},
			httpStubs: forkPost,
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git clone https://github.com/someone/REPO\.git`, 0, "")
				cs.Register(`git -C REPO remote add upstream https://github\.com/OWNER/REPO\.git`, 0, "")
				cs.Register(`git -C REPO fetch upstream`, 0, "")
				cs.Register(`git -C REPO config --add remote.upstream.gh-resolved base`, 0, "")
			},
			wantErrOut: "someone/REPO already exists\n",
		},
		{
			name: "repo arg nontty clone arg",
			opts: &ForkOptions{
				Repository: "OWNER/REPO",
				Clone:      true,
			},
			httpStubs: forkPost,
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git clone https://github.com/someone/REPO\.git`, 0, "")
				cs.Register(`git -C REPO remote add upstream https://github\.com/OWNER/REPO\.git`, 0, "")
				cs.Register(`git -C REPO fetch upstream`, 0, "")
				cs.Register(`git -C REPO config --add remote.upstream.gh-resolved base`, 0, "")
			},
			wantOut: "https://github.com/someone/REPO\n",
		},
		{
			name: "non tty repo arg with fork-name",
			opts: &ForkOptions{
				Repository: "someone/REPO",
				Clone:      false,
				ForkName:   "NEW_REPO",
			},
			tty: false,
			httpStubs: func(reg *httpmock.Registry) {
				forkResult := `{
					"node_id": "123",
					"name": "REPO",
					"clone_url": "https://github.com/OWNER/REPO.git",
					"created_at": "2011-01-26T19:01:12Z",
					"owner": {
						"login": "OWNER"
					}
				}`
				renameResult := `{
					"node_id": "1234",
					"name": "NEW_REPO",
					"clone_url": "https://github.com/OWNER/NEW_REPO.git",
					"created_at": "2012-01-26T19:01:12Z",
					"owner": {
						"login": "OWNER"
					}
				}`
				reg.Register(
					httpmock.REST("POST", "repos/someone/REPO/forks"),
					httpmock.StringResponse(forkResult))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StringResponse(renameResult))
			},
			wantErrOut: "",
			wantOut:    "https://github.com/OWNER/REPO\n",
		},
		{
			name: "tty repo arg with fork-name",
			opts: &ForkOptions{
				Repository: "someone/REPO",
				Clone:      false,
				ForkName:   "NEW_REPO",
			},
			tty: true,
			httpStubs: func(reg *httpmock.Registry) {
				forkResult := `{
					"node_id": "123",
					"name": "REPO",
					"clone_url": "https://github.com/OWNER/REPO.git",
					"created_at": "2011-01-26T19:01:12Z",
					"owner": {
						"login": "OWNER"
					}
				}`
				renameResult := `{
					"node_id": "1234",
					"name": "NEW_REPO",
					"clone_url": "https://github.com/OWNER/NEW_REPO.git",
					"created_at": "2012-01-26T19:01:12Z",
					"owner": {
						"login": "OWNER"
					}
				}`
				reg.Register(
					httpmock.REST("POST", "repos/someone/REPO/forks"),
					httpmock.StringResponse(forkResult))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StringResponse(renameResult))
			},
			wantErrOut: "✓ Created fork OWNER/REPO\n✓ Renamed fork to OWNER/NEW_REPO\n",
		},
		{
			name: "retries clone up to four times if necessary",
			opts: &ForkOptions{
				Repository: "OWNER/REPO",
				Clone:      true,
				BackOff:    &backoff.ZeroBackOff{},
			},
			httpStubs: forkPost,
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git clone https://github.com/someone/REPO\.git`, 128, "")
				cs.Register(`git clone https://github.com/someone/REPO\.git`, 128, "")
				cs.Register(`git clone https://github.com/someone/REPO\.git`, 128, "")
				cs.Register(`git clone https://github.com/someone/REPO\.git`, 0, "")
				cs.Register(`git -C REPO remote add upstream https://github\.com/OWNER/REPO\.git`, 0, "")
				cs.Register(`git -C REPO fetch upstream`, 0, "")
				cs.Register(`git -C REPO config --add remote.upstream.gh-resolved base`, 0, "")
			},
			wantOut: "https://github.com/someone/REPO\n",
		},
		{
			name: "does not retry clone if error occurs and exit code is not 128",
			opts: &ForkOptions{
				Repository: "OWNER/REPO",
				Clone:      true,
				BackOff:    &backoff.ZeroBackOff{},
			},
			httpStubs: forkPost,
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git clone https://github.com/someone/REPO\.git`, 128, "")
				cs.Register(`git clone https://github.com/someone/REPO\.git`, 65, "")
			},
			wantErr: true,
			errMsg:  `failed to clone fork: failed to run git: git -c credential.helper= -c credential.helper=!"[^"]+" auth git-credential clone https://github.com/someone/REPO\.git exited with status 65`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
			tt.opts.IO = ios

			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}

			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			cfg, _ := config.NewIsolatedTestConfig(t)
			if tt.cfgStubs != nil {
				tt.cfgStubs(t, cfg)
			}
			tt.opts.Config = func() (gh.Config, error) {
				return cfg, nil
			}

			tt.opts.Remotes = func() (context.Remotes, error) {
				if tt.remotes == nil {
					return []*context.Remote{
						{
							Remote: &git.Remote{
								Name:     "origin",
								FetchURL: &url.URL{},
							},
							Repo: ghrepo.New("OWNER", "REPO"),
						},
					}, nil
				}
				return tt.remotes, nil
			}

			tt.opts.GitClient = &git.Client{
				GhPath:  "some/path/gh",
				GitPath: "some/path/git",
			}

			pm := prompter.NewMockPrompter(t)
			tt.opts.Prompter = pm
			if tt.promptStubs != nil {
				tt.promptStubs(pm)
			}

			cs, restoreRun := run.Stub()
			defer restoreRun(t)
			if tt.execStubs != nil {
				tt.execStubs(cs)
			}

			if tt.opts.Since == nil {
				tt.opts.Since = func(t time.Time) time.Duration {
					return 2 * time.Second
				}
			}

			defer reg.Verify(t)
			err := forkRun(tt.opts)
			if tt.wantErr {
				assert.Error(t, err, tt.errMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantErrOut, stderr.String())
		})
	}
}
