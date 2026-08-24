package sync

import (
	"github.com/cli/cli/v2/git"
	"github.com/stretchr/testify/mock"
)

type mockGitClient struct {
	mock.Mock
}

func (g *mockGitClient) UpdateBranch(b, r string) error {
	args := g.Called(b, r)
	return args.Error(0)
}

func (g *mockGitClient) Worktrees() ([]git.Worktree, error) {
	args := g.Called()
	return args.Get(0).([]git.Worktree), args.Error(1)
}

func (g *mockGitClient) CreateBranch(b, r, u string) error {
	args := g.Called(b, r, u)
	return args.Error(0)
}

func (g *mockGitClient) CurrentBranch() (string, error) {
	args := g.Called()
	return args.String(0), args.Error(1)
}

func (g *mockGitClient) Fetch(a, b string) error {
	args := g.Called(a, b)
	return args.Error(0)
}

func (g *mockGitClient) HasLocalBranch(a string) bool {
	args := g.Called(a)
	return args.Bool(0)
}

func (g *mockGitClient) IsAncestor(a, b string) (bool, error) {
	args := g.Called(a, b)
	return args.Bool(0), args.Error(1)
}

func (g *mockGitClient) IsDirty() (bool, error) {
	args := g.Called()
	return args.Bool(0), args.Error(1)
}

func (g *mockGitClient) MergeFastForward(a string) error {
	args := g.Called(a)
	return args.Error(0)
}

func (g *mockGitClient) ResetHard(a string) error {
	args := g.Called(a)
	return args.Error(0)
}
