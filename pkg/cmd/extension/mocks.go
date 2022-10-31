package extension

import (
	"os/exec"

	"github.com/cli/cli/v2/git"
	"github.com/stretchr/testify/mock"
)

type mockGitClient struct {
	mock.Mock
}

func (g *mockGitClient) CheckoutBranch(branch string, mods ...git.CommandModifier) error {
	args := g.Called(branch, extractRepoDir(mods))
	return args.Error(0)
}

func (g *mockGitClient) Clone(cloneURL string, cloneArgs []string, mods ...git.CommandModifier) (string, error) {
	args := g.Called(cloneURL, cloneArgs, extractRepoDir(mods))
	return args.String(0), args.Error(1)
}

func (g *mockGitClient) CommandOutput(commandArgs []string, mods ...git.CommandModifier) ([]byte, error) {
	args := g.Called(commandArgs, extractRepoDir(mods))
	return []byte(args.String(0)), args.Error(1)
}

func (g *mockGitClient) Config(name string, mods ...git.CommandModifier) (string, error) {
	args := g.Called(name, extractRepoDir(mods))
	return args.String(0), args.Error(1)
}

func (g *mockGitClient) Fetch(remote string, refspec string, mods ...git.CommandModifier) error {
	args := g.Called(remote, refspec, extractRepoDir(mods))
	return args.Error(0)
}

func (g *mockGitClient) Pull(remote, branch string, mods ...git.CommandModifier) error {
	args := g.Called(remote, branch, extractRepoDir(mods))
	return args.Error(0)
}

func (g *mockGitClient) Remotes(mods ...git.CommandModifier) (git.RemoteSet, error) {
	args := g.Called(extractRepoDir(mods))
	return nil, args.Error(1)
}

func extractRepoDir(mods []git.CommandModifier) string {
	if len(mods) == 0 || len(mods) > 1 {
		return ""
	}
	mod := mods[0]
	gitCmd := &git.Command{Cmd: &exec.Cmd{Args: []string{"-C", ""}}}
	mod(gitCmd)
	return gitCmd.Args[1]
}
