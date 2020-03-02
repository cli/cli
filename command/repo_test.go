package command

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/context"
	"github.com/cli/cli/utils"
)

func TestRepoFork_already_forked(t *testing.T) {
	initContext = func() context.Context {
		ctx := context.NewBlank()
		ctx.SetBaseRepo("REPO")
		ctx.SetBranch("master")
		ctx.SetAuthLogin("someone")
		ctx.SetRemotes(map[string]string{
			"origin": "OWNER/REPO",
		})
		return ctx
	}
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	defer http.StubWithFixture(200, "forkResult.json")()

	output, err := RunCommand(repoForkCmd, "repo fork --remote=false")
	if err != nil {
		t.Errorf("got unexpected error: %v", err)
	}
	r := regexp.MustCompile(`someone/REPO already exists`)
	if !r.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
		return
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

func TestRepoFork_in_parent(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	defer http.StubWithFixture(200, "forkResult.json")()

	output, err := RunCommand(repoForkCmd, "repo fork --remote=false")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	eq(t, output.Stderr(), "")

	expectedLines := []*regexp.Regexp{
		regexp.MustCompile(`Created fork someone/REPO`),
	}
	for _, r := range expectedLines {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestRepoFork_outside(t *testing.T) {
	tests := []struct {
		name string
		args string
	}{
		{
			name: "url arg",
			args: "repo fork --clone=false http://github.com/OWNER/REPO.git",
		},
		{
			name: "full name arg",
			args: "repo fork --clone=false OWNER/REPO",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer stubSince(2 * time.Second)()
			http := initFakeHTTP()
			defer http.StubWithFixture(200, "forkResult.json")()

			output, err := RunCommand(repoForkCmd, tt.args)
			if err != nil {
				t.Errorf("error running command `repo fork`: %v", err)
			}

			eq(t, output.Stderr(), "")

			expectedLines := []*regexp.Regexp{
				regexp.MustCompile(`Created fork someone/REPO`),
			}
			for _, r := range expectedLines {
				if !r.MatchString(output.String()) {
					t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
					return
				}
			}
		})
	}
}

func TestRepoFork_in_parent_yes(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	defer http.StubWithFixture(200, "forkResult.json")()

	var seenCmds []*exec.Cmd
	defer utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmds = append(seenCmds, cmd)
		return &outputStub{}
	})()

	output, err := RunCommand(repoForkCmd, "repo fork --remote")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	expectedCmds := []string{
		"git remote add -f fork https://github.com/someone/repo.git",
		"git fetch fork",
	}

	for x, cmd := range seenCmds {
		eq(t, strings.Join(cmd.Args, " "), expectedCmds[x])
	}

	eq(t, output.Stderr(), "")

	expectedLines := []*regexp.Regexp{
		regexp.MustCompile(`Created fork someone/REPO`),
		regexp.MustCompile(`Remote added at fork`),
	}
	for _, r := range expectedLines {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestRepoFork_outside_yes(t *testing.T) {
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	defer http.StubWithFixture(200, "forkResult.json")()

	var seenCmd *exec.Cmd
	defer utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmd = cmd
		return &outputStub{}
	})()

	output, err := RunCommand(repoForkCmd, "repo fork --clone OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	eq(t, output.Stderr(), "")

	eq(t, strings.Join(seenCmd.Args, " "), "git clone https://github.com/someone/repo.git")

	expectedLines := []*regexp.Regexp{
		regexp.MustCompile(`Created fork someone/REPO`),
		regexp.MustCompile(`Cloned fork`),
	}
	for _, r := range expectedLines {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestRepoFork_outside_survey_yes(t *testing.T) {
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	defer http.StubWithFixture(200, "forkResult.json")()

	var seenCmd *exec.Cmd
	defer utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmd = cmd
		return &outputStub{}
	})()

	oldConfirm := Confirm
	Confirm = func(_ string, result *bool) error {
		*result = true
		return nil
	}
	defer func() { Confirm = oldConfirm }()

	output, err := RunCommand(repoForkCmd, "repo fork OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	eq(t, output.Stderr(), "")

	eq(t, strings.Join(seenCmd.Args, " "), "git clone https://github.com/someone/repo.git")

	expectedLines := []*regexp.Regexp{
		regexp.MustCompile(`Created fork someone/REPO`),
		regexp.MustCompile(`Cloned fork`),
	}
	for _, r := range expectedLines {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestRepoFork_outside_survey_no(t *testing.T) {
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	defer http.StubWithFixture(200, "forkResult.json")()

	cmdRun := false
	defer utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		cmdRun = true
		return &outputStub{}
	})()

	oldConfirm := Confirm
	Confirm = func(_ string, result *bool) error {
		*result = false
		return nil
	}
	defer func() { Confirm = oldConfirm }()

	output, err := RunCommand(repoForkCmd, "repo fork OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	eq(t, output.Stderr(), "")

	eq(t, cmdRun, false)

	expectedLines := []*regexp.Regexp{
		regexp.MustCompile(`Created fork someone/REPO`),
	}
	for _, r := range expectedLines {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestRepoFork_in_parent_survey_yes(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	defer http.StubWithFixture(200, "forkResult.json")()

	var seenCmds []*exec.Cmd
	defer utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmds = append(seenCmds, cmd)
		return &outputStub{}
	})()

	oldConfirm := Confirm
	Confirm = func(_ string, result *bool) error {
		*result = true
		return nil
	}
	defer func() { Confirm = oldConfirm }()

	output, err := RunCommand(repoForkCmd, "repo fork")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	expectedCmds := []string{
		"git remote add -f fork https://github.com/someone/repo.git",
		"git fetch fork",
	}

	for x, cmd := range seenCmds {
		eq(t, strings.Join(cmd.Args, " "), expectedCmds[x])
	}

	eq(t, output.Stderr(), "")

	expectedLines := []*regexp.Regexp{
		regexp.MustCompile(`Created fork someone/REPO`),
		regexp.MustCompile(`Remote added at fork`),
	}
	for _, r := range expectedLines {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestRepoFork_in_parent_survey_no(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	defer stubSince(2 * time.Second)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	defer http.StubWithFixture(200, "forkResult.json")()

	cmdRun := false
	defer utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		cmdRun = true
		return &outputStub{}
	})()

	oldConfirm := Confirm
	Confirm = func(_ string, result *bool) error {
		*result = false
		return nil
	}
	defer func() { Confirm = oldConfirm }()

	output, err := RunCommand(repoForkCmd, "repo fork")
	if err != nil {
		t.Errorf("error running command `repo fork`: %v", err)
	}

	eq(t, output.Stderr(), "")

	eq(t, cmdRun, false)

	expectedLines := []*regexp.Regexp{
		regexp.MustCompile(`Created fork someone/REPO`),
	}
	for _, r := range expectedLines {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestRepoClone(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		{
			name: "shorthand",
			args: "repo clone OWNER/REPO",
			want: "git clone https://github.com/OWNER/REPO.git",
		},
		{
			name: "clone arguments",
			args: "repo clone OWNER/REPO -- -o upstream --depth 1",
			want: "git clone -o upstream --depth 1 https://github.com/OWNER/REPO.git",
		},
		{
			name: "HTTPS URL",
			args: "repo clone https://github.com/OWNER/REPO",
			want: "git clone https://github.com/OWNER/REPO",
		},
		{
			name: "SSH URL",
			args: "repo clone git@github.com:OWNER/REPO.git",
			want: "git clone git@github.com:OWNER/REPO.git",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seenCmd *exec.Cmd
			restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
				seenCmd = cmd
				return &outputStub{}
			})
			defer restoreCmd()

			output, err := RunCommand(repoViewCmd, tt.args)
			if err != nil {
				t.Fatalf("error running command `repo clone`: %v", err)
			}

			eq(t, output.String(), "")
			eq(t, output.Stderr(), "")

			if seenCmd == nil {
				t.Fatal("expected a command to run")
			}
			eq(t, strings.Join(seenCmd.Args, " "), tt.want)
		})
	}
}

func TestRepoView(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	var seenCmd *exec.Cmd
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmd = cmd
		return &outputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(repoViewCmd, "repo view")
	if err != nil {
		t.Errorf("error running command `repo view`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/OWNER/REPO in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO")
}

func TestRepoView_ownerRepo(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	initContext = func() context.Context {
		return ctx
	}
	initFakeHTTP()

	var seenCmd *exec.Cmd
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmd = cmd
		return &outputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(repoViewCmd, "repo view cli/cli")
	if err != nil {
		t.Errorf("error running command `repo view`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/cli/cli in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/cli/cli")
}

func TestRepoView_fullURL(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	initContext = func() context.Context {
		return ctx
	}
	initFakeHTTP()

	var seenCmd *exec.Cmd
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmd = cmd
		return &outputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(repoViewCmd, "repo view https://github.com/cli/cli")
	if err != nil {
		t.Errorf("error running command `repo view`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/cli/cli in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/cli/cli")
}
