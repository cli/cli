package command

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/cli/cli/context"
	"github.com/cli/cli/utils"
)

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
