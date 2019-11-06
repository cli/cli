package command

import (
	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/utils"
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

func mockOpenInBrowser() (func(), *int) {
	callCount := 0
	originalOpenInBrowser := utils.OpenInBrowser
	teardown := func() {
		utils.OpenInBrowser = originalOpenInBrowser
	}

	utils.OpenInBrowser = func(_ string) error {
		callCount++
		return nil
	}

	return teardown, &callCount
}
