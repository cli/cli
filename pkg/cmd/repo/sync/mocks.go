package sync

import (
	"github.com/stretchr/testify/mock"
)

type mockGitClient struct {
	mock.Mock
}

func (g *mockGitClient) Checkout(a []string) error {
	args := g.Called(a)
	return args.Error(0)
}

func (g *mockGitClient) CurrentBranch() (string, error) {
	args := g.Called()
	return args.String(0), args.Error(1)
}

func (g *mockGitClient) Fetch(a []string) error {
	args := g.Called(a)
	return args.Error(0)
}

func (g *mockGitClient) HasLocalBranch(a []string) bool {
	args := g.Called(a)
	return args.Bool(0)
}

func (g *mockGitClient) IsAncestor(a []string) (bool, error) {
	args := g.Called(a)
	return args.Bool(0), args.Error(1)
}

func (g *mockGitClient) IsDirty() (bool, error) {
	args := g.Called()
	return args.Bool(0), args.Error(1)
}

func (g *mockGitClient) Merge(a []string) error {
	args := g.Called(a)
	return args.Error(0)
}

func (g *mockGitClient) Reset(a []string) error {
	args := g.Called(a)
	return args.Error(0)
}
