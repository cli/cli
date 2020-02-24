package command

import (
	"os/exec"
	"testing"

	"github.com/cli/cli/context"
	"github.com/cli/cli/utils"
)

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
	eq(t, output.Stderr(), "Opening https://github.com/OWNER/REPO in your browser.\n")

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
	eq(t, output.Stderr(), "Opening https://github.com/cli/cli in your browser.\n")

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
	eq(t, output.Stderr(), "Opening https://github.com/cli/cli in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/cli/cli")
}
