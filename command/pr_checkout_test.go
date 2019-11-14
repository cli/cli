package command

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/utils"
)

func TestPRCheckout_sameRepo(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	ctx.SetRemotes(map[string]string{
		"origin": "OWNER/REPO",
	})
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "REPO",
			"defaultBranchRef": {
				"name": "master"
			}
		},
		"isCrossRepository": false,
		"maintainerCanModify": false
	} } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		ranCommands = append(ranCommands, cmd.Args)
		return &outputStub{}
	})
	defer restoreCmd()

	RootCmd.SetArgs([]string{"pr", "checkout", "123"})
	_, err := prCheckoutCmd.ExecuteC()
	eq(t, err, nil)

	eq(t, len(ranCommands), 6)
	eq(t, strings.Join(ranCommands[0], " "), "git rev-parse -q --git-path refs/heads/feature")
	eq(t, strings.Join(ranCommands[1], " "), "git rev-parse -q --git-dir")
	eq(t, strings.Join(ranCommands[2], " "), "git fetch origin +refs/heads/feature:refs/remotes/origin/feature")
	eq(t, strings.Join(ranCommands[3], " "), "git checkout -b feature --no-track origin/feature")
	eq(t, strings.Join(ranCommands[4], " "), "git config branch.feature.remote origin")
	eq(t, strings.Join(ranCommands[5], " "), "git config branch.feature.merge refs/heads/feature")
}

func TestPRCheckout_differentRepo(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	ctx.SetRemotes(map[string]string{
		"origin": "OWNER/REPO",
	})
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "REPO",
			"defaultBranchRef": {
				"name": "master"
			}
		},
		"isCrossRepository": true,
		"maintainerCanModify": false
	} } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		ranCommands = append(ranCommands, cmd.Args)
		return &outputStub{}
	})
	defer restoreCmd()

	RootCmd.SetArgs([]string{"pr", "checkout", "123"})
	_, err := prCheckoutCmd.ExecuteC()
	eq(t, err, nil)

	eq(t, len(ranCommands), 5)
	eq(t, strings.Join(ranCommands[0], " "), "git config branch.feature.merge")
	eq(t, strings.Join(ranCommands[1], " "), "git fetch origin refs/pull/123/head:feature")
	eq(t, strings.Join(ranCommands[2], " "), "git checkout feature")
	eq(t, strings.Join(ranCommands[3], " "), "git config branch.feature.remote origin")
	eq(t, strings.Join(ranCommands[4], " "), "git config branch.feature.merge refs/pull/123/head")
}

func TestPRCheckout_maintainerCanModify(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	ctx.SetRemotes(map[string]string{
		"origin": "OWNER/REPO",
	})
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "REPO",
			"defaultBranchRef": {
				"name": "master"
			}
		},
		"isCrossRepository": true,
		"maintainerCanModify": true
	} } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		ranCommands = append(ranCommands, cmd.Args)
		return &outputStub{}
	})
	defer restoreCmd()

	RootCmd.SetArgs([]string{"pr", "checkout", "123"})
	_, err := prCheckoutCmd.ExecuteC()
	eq(t, err, nil)

	eq(t, len(ranCommands), 5)
	eq(t, strings.Join(ranCommands[0], " "), "git config branch.feature.merge")
	eq(t, strings.Join(ranCommands[1], " "), "git fetch origin refs/pull/123/head:feature")
	eq(t, strings.Join(ranCommands[2], " "), "git checkout feature")
	eq(t, strings.Join(ranCommands[3], " "), "git config branch.feature.remote https://github.com/hubot/REPO.git")
	eq(t, strings.Join(ranCommands[4], " "), "git config branch.feature.merge refs/heads/feature")
}
