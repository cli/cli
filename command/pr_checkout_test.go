package command

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os/exec"
	"strings"
	"testing"

	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/test"
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
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
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
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/pr/123":
			return &errorStub{"exit status: 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := RunCommand(`pr checkout 123`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 4)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin +refs/heads/feature:refs/remotes/origin/feature")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout -b pr/123 --no-track refs/remotes/origin/feature")
	eq(t, strings.Join(ranCommands[2], " "), "git config branch.pr/123.remote origin")
	eq(t, strings.Join(ranCommands[3], " "), "git config branch.pr/123.merge refs/heads/feature")
}

func TestPRCheckout_urlArg(t *testing.T) {
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
		"number": 123,
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
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/pr/123":
			return &errorStub{"exit status: 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := RunCommand(`pr checkout https://github.com/OWNER/REPO/pull/123/files`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 4)
	eq(t, strings.Join(ranCommands[1], " "), "git checkout -b pr/123 --no-track refs/remotes/origin/feature")
}

func TestPRCheckout_urlArg_differentBase(t *testing.T) {
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
		"number": 123,
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "POE",
			"defaultBranchRef": {
				"name": "master"
			}
		},
		"isCrossRepository": false,
		"maintainerCanModify": false
	} } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/pr/123":
			return &errorStub{"exit status: 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := RunCommand(`pr checkout https://github.com/OTHER/POE/pull/123/files`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	reqBody := struct {
		Variables struct {
			Owner string
			Repo  string
		}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Owner, "OTHER")
	eq(t, reqBody.Variables.Repo, "POE")

	eq(t, len(ranCommands), 5)
	eq(t, strings.Join(ranCommands[1], " "), "git fetch https://github.com/OTHER/POE.git refs/pull/123/head:pr/123")
	eq(t, strings.Join(ranCommands[3], " "), "git config branch.pr/123.remote https://github.com/OTHER/POE.git")
}

func TestPRCheckout_branchArg(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	ctx.SetRemotes(map[string]string{
		"origin": "OWNER/REPO",
	})
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequests": { "nodes": [
		{ "number": 123,
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
		  "maintainerCanModify": false }
	] } } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/pr/123":
			return &errorStub{"exit status: 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := RunCommand(`pr checkout hubot:feature`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 5)
	eq(t, strings.Join(ranCommands[1], " "), "git fetch origin refs/pull/123/head:pr/123")
}

func TestPRCheckout_existingBranch(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	ctx.SetRemotes(map[string]string{
		"origin": "OWNER/REPO",
	})
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
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
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/pr/123":
			return &test.OutputStub{}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := RunCommand(`pr checkout 123`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 3)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin +refs/heads/feature:refs/remotes/origin/feature")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout pr/123")
	eq(t, strings.Join(ranCommands[2], " "), "git merge --ff-only refs/remotes/origin/feature")
}

func TestPRCheckout_differentRepo_remoteExists(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	ctx.SetRemotes(map[string]string{
		"origin":     "OWNER/REPO",
		"robot-fork": "hubot/REPO",
	})
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
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
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/pr/123":
			return &errorStub{"exit status: 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := RunCommand(`pr checkout 123`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 4)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch robot-fork +refs/heads/feature:refs/remotes/robot-fork/feature")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout -b pr/123 --no-track refs/remotes/robot-fork/feature")
	eq(t, strings.Join(ranCommands[2], " "), "git config branch.pr/123.remote robot-fork")
	eq(t, strings.Join(ranCommands[3], " "), "git config branch.pr/123.merge refs/heads/feature")
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
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
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
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git config branch.pr/123.merge":
			return &errorStub{"exit status 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := RunCommand(`pr checkout 123`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 4)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin refs/pull/123/head:pr/123")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout pr/123")
	eq(t, strings.Join(ranCommands[2], " "), "git config branch.pr/123.remote origin")
	eq(t, strings.Join(ranCommands[3], " "), "git config branch.pr/123.merge refs/pull/123/head")
}

func TestPRCheckout_differentRepo_existingBranch(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	ctx.SetRemotes(map[string]string{
		"origin": "OWNER/REPO",
	})
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
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
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git config branch.pr/123.merge":
			return &test.OutputStub{Out: []byte("refs/heads/feature\n")}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := RunCommand(`pr checkout 123`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 2)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin refs/pull/123/head:pr/123")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout pr/123")
}

func TestPRCheckout_differentRepo_currentBranch(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("pr/123")
	ctx.SetRemotes(map[string]string{
		"origin": "OWNER/REPO",
	})
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
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
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git config branch.pr/123.merge":
			return &test.OutputStub{Out: []byte("refs/heads/feature\n")}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := RunCommand(`pr checkout 123`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 2)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin refs/pull/123/head")
	eq(t, strings.Join(ranCommands[1], " "), "git merge --ff-only FETCH_HEAD")
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
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
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
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git config branch.pr/123.merge":
			return &errorStub{"exit status 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := RunCommand(`pr checkout 123`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 4)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin refs/pull/123/head:pr/123")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout pr/123")
	eq(t, strings.Join(ranCommands[2], " "), "git config branch.pr/123.remote https://github.com/hubot/REPO.git")
	eq(t, strings.Join(ranCommands[3], " "), "git config branch.pr/123.merge refs/heads/feature")
}

func TestPRCheckout_withBranchFlag(t *testing.T) {
	// Just need to test it with any one of the use cases. That the value passed is being respected
	ctx := context.NewBlank()
	ctx.SetBranch("master")
	ctx.SetRemotes(map[string]string{
		"origin": "OWNER/REPO",
	})
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
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
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/abc":
			return &errorStub{"exit status: 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := RunCommand(`pr checkout 123 -b abc`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 4)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin +refs/heads/feature:refs/remotes/origin/feature")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout -b abc --no-track refs/remotes/origin/feature")
	eq(t, strings.Join(ranCommands[2], " "), "git config branch.abc.remote origin")
	eq(t, strings.Join(ranCommands[3], " "), "git config branch.abc.merge refs/heads/feature")
}
