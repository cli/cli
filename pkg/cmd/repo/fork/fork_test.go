package fork

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/test"
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
	}{
		{
			name: "repo with git args",
			cli:  "foo/bar -- --foo=bar",
			wants: ForkOptions{
				Repository: "foo/bar",
				GitArgs:    []string{"TODO"},
				RemoteName: "origin",
				Rename:     true,
			},
		},
		{
			name:    "git args without repo",
			cli:     "-- --foo bar",
			wantErr: true,
		},
		{
			name: "repo",
			cli:  "foo/bar",
			wants: ForkOptions{
				Repository: "foo/bar",
				RemoteName: "origin",
				Rename:     true,
			},
		},
		{
			name:    "blank remote name",
			cli:     "--remote --remote-name=''",
			wantErr: true,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			io.SetStdoutTTY(tt.tty)
			io.SetStdinTTY(tt.tty)

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
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.RemoteName, gotOpts.RemoteName)
			assert.Equal(t, tt.wants.Remote, gotOpts.Remote)
			assert.Equal(t, tt.wants.PromptRemote, gotOpts.PromptRemote)
			assert.Equal(t, tt.wants.PromptClone, gotOpts.PromptClone)
			assert.Equal(t, tt.wants.Organization, gotOpts.Organization)
		})
	}
}

func runCommand(httpClient *http.Client, remotes []*context.Remote, isTTY bool, cli string) (*test.CmdOut, error) {
	io, stdin, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(isTTY)
	io.SetStdinTTY(isTTY)
	io.SetStderrTTY(isTTY)
	fac := &cmdutil.Factory{
		IOStreams: io,
		HttpClient: func() (*http.Client, error) {
			return httpClient, nil
		},
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		Remotes: func() (context.Remotes, error) {
			if remotes == nil {
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

			return remotes, nil
		},
	}

	cmd := NewCmdFork(fac, nil)

	argv, err := shlex.Split(cli)
	cmd.SetArgs(argv)

	cmd.SetIn(stdin)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	if err != nil {
		panic(err)
	}

	_, err = cmd.ExecuteC()

	if err != nil {
		return nil, err
	}

	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr}, nil
}

func TestRepoFork_nontty(t *testing.T) {
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	_, restore := run.Stub()
	defer restore(t)

	output, err := runCommand(httpClient, nil, false, "")
	if err != nil {
		t.Fatalf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())

}

func TestRepoFork_no_conflicting_remote(t *testing.T) {
	remotes := []*context.Remote{
		{
			Remote: &git.Remote{
				Name:     "upstream",
				FetchURL: &url.URL{},
			},
			Repo: ghrepo.New("OWNER", "REPO"),
		},
	}
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := run.Stub()
	defer restore(t)

	cs.Register(`git remote add -f origin https://github\.com/someone/REPO\.git`, 0, "")

	output, err := runCommand(httpClient, remotes, false, "--remote")
	if err != nil {
		t.Fatalf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestRepoFork_existing_remote_error(t *testing.T) {
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	_, err := runCommand(httpClient, nil, true, "--remote --remote-name='origin'")
	if err == nil {
		t.Fatal("expected error running command `repo fork`")
	}

	assert.Equal(t, "a git remote named 'origin' already exists", err.Error())

	reg.Verify(t)
}

func TestRepoFork_in_parent_tty(t *testing.T) {
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := run.Stub()
	defer restore(t)

	cs.Register("git remote rename origin upstream", 0, "")
	cs.Register(`git remote add -f origin https://github\.com/someone/REPO\.git`, 0, "")

	output, err := runCommand(httpClient, nil, true, "--remote")
	if err != nil {
		t.Fatalf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Created fork someone/REPO\n✓ Added remote origin\n", output.Stderr())
	reg.Verify(t)
}

func TestRepoFork_in_parent_nontty(t *testing.T) {
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := run.Stub()
	defer restore(t)

	cs.Register("git remote rename origin upstream", 0, "")
	cs.Register(`git remote add -f origin https://github\.com/someone/REPO\.git`, 0, "")

	output, err := runCommand(httpClient, nil, false, "--remote")
	if err != nil {
		t.Fatalf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
	reg.Verify(t)
}

func TestRepoFork_outside_parent_nontty(t *testing.T) {
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	reg.Verify(t)
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := run.Stub()
	defer restore(t)

	cs.Register(`git clone https://github.com/someone/REPO\.git`, 0, "")
	cs.Register(`git -C REPO remote add -f upstream https://github\.com/OWNER/REPO\.git`, 0, "")

	output, err := runCommand(httpClient, nil, false, "--clone OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, output.Stderr(), "")

}

func TestRepoFork_already_forked(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	_, restore := run.Stub()
	defer restore(t)

	output, err := runCommand(httpClient, nil, true, "--remote=false")
	if err != nil {
		t.Errorf("got unexpected error: %v", err)
	}

	r := regexp.MustCompile(`someone/REPO.*already exists`)
	if !r.MatchString(output.Stderr()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output.Stderr())
		return
	}

	reg.Verify(t)
}

func TestRepoFork_reuseRemote(t *testing.T) {
	remotes := []*context.Remote{
		{
			Remote: &git.Remote{Name: "origin", FetchURL: &url.URL{}},
			Repo:   ghrepo.New("someone", "REPO"),
		},
		{
			Remote: &git.Remote{Name: "upstream", FetchURL: &url.URL{}},
			Repo:   ghrepo.New("OWNER", "REPO"),
		},
	}
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	output, err := runCommand(httpClient, remotes, true, "--remote")
	if err != nil {
		t.Errorf("got unexpected error: %v", err)
	}
	r := regexp.MustCompile(`Using existing remote.*origin`)
	if !r.MatchString(output.Stderr()) {
		t.Errorf("output did not match: %q", output.Stderr())
		return
	}
	reg.Verify(t)
}

func TestRepoFork_in_parent(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	_, restore := run.Stub()
	defer restore(t)
	defer stubSince(2 * time.Second)()

	output, err := runCommand(httpClient, nil, true, "--remote=false")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())

	r := regexp.MustCompile(`Created fork.*someone/REPO`)
	if !r.MatchString(output.Stderr()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
		return
	}
	reg.Verify(t)
}

func TestRepoFork_outside(t *testing.T) {
	tests := []struct {
		name         string
		args         string
		postBody     string
		responseBody string
		wantStderr   string
	}{
		{
			name:         "url arg",
			args:         "--clone=false http://github.com/OWNER/REPO.git",
			postBody:     "{}\n",
			responseBody: `{"name":"REPO", "owner":{"login":"monalisa"}}`,
			wantStderr: heredoc.Doc(`
				✓ Created fork monalisa/REPO
			`),
		},
		{
			name:         "full name arg",
			args:         "--clone=false OWNER/REPO",
			postBody:     "{}\n",
			responseBody: `{"name":"REPO", "owner":{"login":"monalisa"}}`,
			wantStderr: heredoc.Doc(`
				✓ Created fork monalisa/REPO
			`),
		},
		{
			name:         "fork to org without clone",
			args:         "--clone=false OWNER/REPO --org batmanshome",
			postBody:     "{\"organization\":\"batmanshome\"}\n",
			responseBody: `{"name":"REPO", "owner":{"login":"BatmansHome"}}`,
			wantStderr: heredoc.Doc(`
				✓ Created fork BatmansHome/REPO
			`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer stubSince(2 * time.Second)()

			reg := &httpmock.Registry{}
			reg.Register(
				httpmock.REST("POST", "repos/OWNER/REPO/forks"),
				func(req *http.Request) (*http.Response, error) {
					bb, err := ioutil.ReadAll(req.Body)
					if err != nil {
						return nil, err
					}
					assert.Equal(t, tt.postBody, string(bb))
					return &http.Response{
						Request:    req,
						StatusCode: 200,
						Body:       ioutil.NopCloser(bytes.NewBufferString(tt.responseBody)),
					}, nil
				})

			httpClient := &http.Client{Transport: reg}
			output, err := runCommand(httpClient, nil, true, tt.args)
			assert.NoError(t, err)
			assert.Equal(t, "", output.String())
			assert.Equal(t, tt.wantStderr, output.Stderr())
			reg.Verify(t)
		})
	}
}

func TestRepoFork_in_parent_yes(t *testing.T) {
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := run.Stub()
	defer restore(t)

	cs.Register(`git remote add -f fork https://github\.com/someone/REPO\.git`, 0, "")

	output, err := runCommand(httpClient, nil, true, "--remote --remote-name=fork")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())
	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(),
		"Created fork.*someone/REPO",
		"Added remote.*fork")
	reg.Verify(t)
}

func TestRepoFork_outside_yes(t *testing.T) {
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := run.Stub()
	defer restore(t)

	cs.Register(`git clone https://github\.com/someone/REPO\.git`, 0, "")
	cs.Register(`git -C REPO remote add -f upstream https://github\.com/OWNER/REPO\.git`, 0, "")

	output, err := runCommand(httpClient, nil, true, "--clone OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())
	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(),
		"Created fork.*someone/REPO",
		"Cloned fork")
	reg.Verify(t)
}

func TestRepoFork_ForkAlreadyExistsAndCloneNonTty(t *testing.T) {
	defer stubSince(2 * time.Minute)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := run.Stub()
	defer restore(t)

	cs.Register(`git clone https://github\.com/someone/REPO\.git`, 0, "")
	cs.Register(`git -C REPO remote add -f upstream https://github\.com/OWNER/REPO\.git`, 0, "")

	output, err := runCommand(httpClient, nil, false, "--clone OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())
	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(),
		"someone/REPO.*already exists")
	reg.Verify(t)
}

func TestRepoFork_outside_survey_yes(t *testing.T) {
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := run.Stub()
	defer restore(t)

	cs.Register(`git clone https://github\.com/someone/REPO\.git`, 0, "")
	cs.Register(`git -C REPO remote add -f upstream https://github\.com/OWNER/REPO\.git`, 0, "")

	defer prompt.StubConfirm(true)()

	output, err := runCommand(httpClient, nil, true, "OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())
	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(),
		"Created fork.*someone/REPO",
		"Cloned fork")
	reg.Verify(t)
}

func TestRepoFork_outside_survey_no(t *testing.T) {
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	_, restore := run.Stub()
	defer restore(t)

	defer prompt.StubConfirm(false)()

	output, err := runCommand(httpClient, nil, true, "OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())

	r := regexp.MustCompile(`Created fork.*someone/REPO`)
	if !r.MatchString(output.Stderr()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
		return
	}
	reg.Verify(t)
}

func TestRepoFork_in_parent_survey_yes(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}
	defer stubSince(2 * time.Second)()

	cs, restore := run.Stub()
	defer restore(t)

	cs.Register(`git remote add -f fork https://github\.com/someone/REPO\.git`, 0, "")

	defer prompt.StubConfirm(true)()

	output, err := runCommand(httpClient, nil, true, "--remote-name=fork")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(),
		"Created fork.*someone/REPO",
		"Added remote.*fork")
	reg.Verify(t)
}

func TestRepoFork_in_parent_survey_no(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}
	defer stubSince(2 * time.Second)()

	_, restore := run.Stub()
	defer restore(t)

	defer prompt.StubConfirm(false)()

	output, err := runCommand(httpClient, nil, true, "")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())

	r := regexp.MustCompile(`Created fork.*someone/REPO`)
	if !r.MatchString(output.Stderr()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
		return
	}
}

func Test_RepoFork_gitFlags(t *testing.T) {
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git clone --depth 1 https://github.com/someone/REPO.git`, 0, "")
	cs.Register(`git -C REPO remote add -f upstream https://github.com/OWNER/REPO.git`, 0, "")

	output, err := runCommand(httpClient, nil, false, "--clone OWNER/REPO -- --depth 1")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, output.Stderr(), "")
	reg.Verify(t)
}

func Test_RepoFork_flagError(t *testing.T) {
	_, err := runCommand(nil, nil, true, "--depth 1 OWNER/REPO")
	if err == nil || err.Error() != "unknown flag: --depth\nSeparate git clone flags with '--'." {
		t.Errorf("unexpected error %v", err)
	}
}

func TestRepoFork_in_parent_match_protocol(t *testing.T) {
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := run.Stub()
	defer restore(t)

	cs.Register(`git remote add -f fork git@github\.com:someone/REPO\.git`, 0, "")

	// TODO this breaks, i assume, because as far as this test is concerned "https" has been explicitly configured by the user.

	// TODO split this into multiple explicit test cases for:
	// - user has no protocol configured
	// - user has host protocol configured that differs from global and from arguments
	// - user has protocol configured that matches arguments

	remotes := []*context.Remote{
		{
			Remote: &git.Remote{Name: "origin", PushURL: &url.URL{
				Scheme: "ssh",
			}},
			Repo: ghrepo.New("OWNER", "REPO"),
		},
	}

	output, err := runCommand(httpClient, remotes, true, "--remote --remote-name=fork")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(),
		"Created fork.*someone/REPO",
		"Added remote.*fork")
}

// TODO try to replace with a Now in the opts
func stubSince(d time.Duration) func() {
	originalSince := Since
	Since = func(t time.Time) time.Duration {
		return d
	}
	return func() {
		Since = originalSince
	}
}

func TestRepoFork_in_parent_no_protocol_configured(t *testing.T) {
}

func TestRepoFork(t *testing.T) {
	tests := []struct {
		name      string
		opts      *ForkOptions
		cfg       func(*config.Config)
		tty       bool
		httpStubs func(*httpmock.Registry)
		execStubs func(*run.COmmandStubber)
		remotes   []*context.Remote
		wantOut   string
		wantErr   string
	}{
		// TODO in parent no protocol configured
		// TODO in parent protocol configured
		// TODO in parent nontty
		// TODO in parent existing remote error

	}

	for _, tt := range tests {
		io, _, stdout, stderr := iostreams.Test()
		io.SetStdinTTY(tt.tty)
		io.SetStdoutTTY(tt.tty)
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

		cfg := config.NewBlankConfig()
		if tt.cfg != nil {
			cfg = tt.cfg(cfg)
		}

		tt.opts.Config = func() (config.Config, error) {
			return cfg
		}

		tt.opts.Remotes = func() (context.Remotes, error) {
			if remotes == nil {
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
			return remotes, nil
		}

		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, 1, 1)
		})
	}
}
