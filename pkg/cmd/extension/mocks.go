package extension

import (
	"github.com/cli/cli/v2/git"
	"github.com/stretchr/testify/mock"
)

type mockGitClient struct {
	mock.Mock
}

func (g *mockGitClient) CheckoutBranch(branch string) error {
	args := g.Called(branch)
	return args.Error(0)
}

func (g *mockGitClient) Clone(cloneURL string, cloneArgs []string) (string, error) {
	args := g.Called(cloneURL, cloneArgs)
	return args.String(0), args.Error(1)
}

func (g *mockGitClient) CommandOutput(commandArgs []string) ([]byte, error) {
	args := g.Called(commandArgs)
	return []byte(args.String(0)), args.Error(1)
}

func (g *mockGitClient) Config(name string) (string, error) {
	args := g.Called(name)
	return args.String(0), args.Error(1)
}

func (g *mockGitClient) Fetch(remote string, refspec string) error {
	args := g.Called(remote, refspec)
	return args.Error(0)
}

func (g *mockGitClient) ForRepo(repoDir string) gitClient {
	args := g.Called(repoDir)
	if v, ok := args.Get(0).(*mockGitClient); ok {
		return v
	}
	return nil
}

func (g *mockGitClient) Pull(remote, branch string) error {
	args := g.Called(remote, branch)
	return args.Error(0)
}

func (g *mockGitClient) Remotes() (git.RemoteSet, error) {
	args := g.Called()
	return nil, args.Error(1)
}
