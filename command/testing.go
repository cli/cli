package command

import (
	"errors"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/context"
)

func initBlankContext(repo, branch string) {
	initContext = func() context.Context {
		ctx := context.NewBlank()
		ctx.SetBaseRepo(repo)
		ctx.SetBranch(branch)
		return ctx
	}
}

func initFakeHTTP() *api.FakeHTTP {
	http := &api.FakeHTTP{}
	apiClientForContext = func(context.Context) (*api.Client, error) {
		return api.NewClient(api.ReplaceTripper(http)), nil
	}
	return http
}

// outputStub implements a simple utils.Runnable
type outputStub struct {
	output []byte
}

func (s outputStub) Output() ([]byte, error) {
	return s.output, nil
}

func (s outputStub) Run() error {
	return nil
}

type errorStub struct {
	message string
}

func (s errorStub) Output() ([]byte, error) {
	return nil, errors.New(s.message)
}

func (s errorStub) Run() error {
	return errors.New(s.message)
}
