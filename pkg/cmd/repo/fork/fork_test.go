package fork

import (
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/briandowns/spinner"
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
	"github.com/cli/cli/utils"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

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
						Remote: &git.Remote{Name: "origin"},
						Repo:   ghrepo.New("OWNER", "REPO"),
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
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := test.InitCmdStubber()
	defer restore()

	output, err := runCommand(httpClient, nil, false, "")
	if err != nil {
		t.Fatalf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, 0, len(cs.Calls))
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
	reg.Verify(t)
}

func TestRepoFork_in_parent_nontty(t *testing.T) {
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := test.InitCmdStubber()
	defer restore()

	cs.Stub("") // git remote rename
	cs.Stub("") // git remote add

	output, err := runCommand(httpClient, nil, false, "--remote")
	if err != nil {
		t.Fatalf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, 2, len(cs.Calls))
	assert.Equal(t, "git remote rename origin upstream", strings.Join(cs.Calls[0].Args, " "))
	assert.Equal(t, "git remote add -f origin https://github.com/someone/REPO.git", strings.Join(cs.Calls[1].Args, " "))

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
	reg.Verify(t)
}

func TestRepoFork_outside_parent_nontty(t *testing.T) {
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := test.InitCmdStubber()
	defer restore()

	cs.Stub("") // git clone
	cs.Stub("") // git remote add

	output, err := runCommand(httpClient, nil, false, "--clone OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())

	assert.Equal(t, "git clone https://github.com/someone/REPO.git", strings.Join(cs.Calls[0].Args, " "))
	assert.Equal(t, "git -C REPO remote add -f upstream https://github.com/OWNER/REPO.git", strings.Join(cs.Calls[1].Args, " "))

	assert.Equal(t, output.Stderr(), "")
	reg.Verify(t)
}

func TestRepoFork_already_forked(t *testing.T) {
	stubSpinner()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := test.InitCmdStubber()
	defer restore()

	output, err := runCommand(httpClient, nil, true, "--remote=false")
	if err != nil {
		t.Errorf("got unexpected error: %v", err)
	}

	assert.Equal(t, 0, len(cs.Calls))

	r := regexp.MustCompile(`someone/REPO.*already exists`)
	if !r.MatchString(output.Stderr()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output.Stderr())
		return
	}

	reg.Verify(t)
}

func TestRepoFork_reuseRemote(t *testing.T) {
	stubSpinner()
	remotes := []*context.Remote{
		{
			Remote: &git.Remote{Name: "origin"},
			Repo:   ghrepo.New("someone", "REPO"),
		},
		{
			Remote: &git.Remote{Name: "upstream"},
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
	stubSpinner()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := test.InitCmdStubber()
	defer restore()
	defer stubSince(2 * time.Second)()

	output, err := runCommand(httpClient, nil, true, "--remote=false")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, 0, len(cs.Calls))
	assert.Equal(t, "", output.String())

	r := regexp.MustCompile(`Created fork.*someone/REPO`)
	if !r.MatchString(output.Stderr()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
		return
	}
	reg.Verify(t)
}

func TestRepoFork_outside(t *testing.T) {
	stubSpinner()
	tests := []struct {
		name string
		args string
	}{
		{
			name: "url arg",
			args: "--clone=false http://github.com/OWNER/REPO.git",
		},
		{
			name: "full name arg",
			args: "--clone=false OWNER/REPO",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer stubSince(2 * time.Second)()
			reg := &httpmock.Registry{}
			defer reg.StubWithFixturePath(200, "./forkResult.json")()
			httpClient := &http.Client{Transport: reg}

			output, err := runCommand(httpClient, nil, true, tt.args)
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
		})
	}
}

func TestRepoFork_in_parent_yes(t *testing.T) {
	stubSpinner()
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	var seenCmds []*exec.Cmd
	defer run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmds = append(seenCmds, cmd)
		return &test.OutputStub{}
	})()

	output, err := runCommand(httpClient, nil, true, "--remote")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	expectedCmds := []string{
		"git remote rename origin upstream",
		"git remote add -f origin https://github.com/someone/REPO.git",
	}

	for x, cmd := range seenCmds {
		assert.Equal(t, expectedCmds[x], strings.Join(cmd.Args, " "))
	}

	assert.Equal(t, "", output.String())

	test.ExpectLines(t, output.Stderr(),
		"Created fork.*someone/REPO",
		"Added remote.*origin")
	reg.Verify(t)
}

func TestRepoFork_outside_yes(t *testing.T) {
	stubSpinner()
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := test.InitCmdStubber()
	defer restore()

	cs.Stub("") // git clone
	cs.Stub("") // git remote add

	output, err := runCommand(httpClient, nil, true, "--clone OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())

	assert.Equal(t, "git clone https://github.com/someone/REPO.git", strings.Join(cs.Calls[0].Args, " "))
	assert.Equal(t, "git -C REPO remote add -f upstream https://github.com/OWNER/REPO.git", strings.Join(cs.Calls[1].Args, " "))

	test.ExpectLines(t, output.Stderr(),
		"Created fork.*someone/REPO",
		"Cloned fork")
	reg.Verify(t)
}

func TestRepoFork_outside_survey_yes(t *testing.T) {
	stubSpinner()
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cs, restore := test.InitCmdStubber()
	defer restore()

	cs.Stub("") // git clone
	cs.Stub("") // git remote add

	defer prompt.StubConfirm(true)()

	output, err := runCommand(httpClient, nil, true, "OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())

	assert.Equal(t, "git clone https://github.com/someone/REPO.git", strings.Join(cs.Calls[0].Args, " "))
	assert.Equal(t, "git -C REPO remote add -f upstream https://github.com/OWNER/REPO.git", strings.Join(cs.Calls[1].Args, " "))

	test.ExpectLines(t, output.Stderr(),
		"Created fork.*someone/REPO",
		"Cloned fork")
	reg.Verify(t)
}

func TestRepoFork_outside_survey_no(t *testing.T) {
	stubSpinner()
	defer stubSince(2 * time.Second)()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}

	cmdRun := false
	defer run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		cmdRun = true
		return &test.OutputStub{}
	})()

	defer prompt.StubConfirm(false)()

	output, err := runCommand(httpClient, nil, true, "OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())

	assert.Equal(t, false, cmdRun)

	r := regexp.MustCompile(`Created fork.*someone/REPO`)
	if !r.MatchString(output.Stderr()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
		return
	}
	reg.Verify(t)
}

func TestRepoFork_in_parent_survey_yes(t *testing.T) {
	stubSpinner()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}
	defer stubSince(2 * time.Second)()

	var seenCmds []*exec.Cmd
	defer run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmds = append(seenCmds, cmd)
		return &test.OutputStub{}
	})()

	defer prompt.StubConfirm(true)()

	output, err := runCommand(httpClient, nil, true, "")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	expectedCmds := []string{
		"git remote rename origin upstream",
		"git remote add -f origin https://github.com/someone/REPO.git",
	}

	for x, cmd := range seenCmds {
		assert.Equal(t, expectedCmds[x], strings.Join(cmd.Args, " "))
	}

	assert.Equal(t, "", output.String())

	test.ExpectLines(t, output.Stderr(),
		"Created fork.*someone/REPO",
		"Renamed.*origin.*remote to.*upstream",
		"Added remote.*origin")
	reg.Verify(t)
}

func TestRepoFork_in_parent_survey_no(t *testing.T) {
	stubSpinner()
	reg := &httpmock.Registry{}
	defer reg.StubWithFixturePath(200, "./forkResult.json")()
	httpClient := &http.Client{Transport: reg}
	defer stubSince(2 * time.Second)()

	cmdRun := false
	defer run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		cmdRun = true
		return &test.OutputStub{}
	})()

	defer prompt.StubConfirm(false)()

	output, err := runCommand(httpClient, nil, true, "")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	assert.Equal(t, "", output.String())

	assert.Equal(t, false, cmdRun)

	r := regexp.MustCompile(`Created fork.*someone/REPO`)
	if !r.MatchString(output.Stderr()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
		return
	}
	reg.Verify(t)
}

func stubSpinner() {
	// not bothering with teardown since we never want spinners when doing tests
	utils.StartSpinner = func(_ *spinner.Spinner) {
	}
	utils.StopSpinner = func(_ *spinner.Spinner) {
	}
}

func stubSince(d time.Duration) func() {
	originalSince := Since
	Since = func(t time.Time) time.Duration {
		return d
	}
	return func() {
		Since = originalSince
	}
}
