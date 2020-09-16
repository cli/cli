package checkout

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
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

func runCommand(rt http.RoundTripper, remotes context.Remotes, branch string, cli string) (*test.CmdOut, error) {
	io, _, stdout, stderr := iostreams.Test()

	factory := &cmdutil.Factory{
		IOStreams: io,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return api.InitRepoHostname(&api.Repository{
				Name:             "REPO",
				Owner:            api.RepositoryOwner{Login: "OWNER"},
				DefaultBranchRef: api.BranchRef{Name: "master"},
			}, "github.com"), nil
		},
		Remotes: func() (context.Remotes, error) {
			if remotes == nil {
				return context.Remotes{
					{
						Remote: &git.Remote{Name: "origin"},
						Repo:   ghrepo.New("OWNER", "REPO"),
					},
				}, nil
			}
			return remotes, nil
		},
		Branch: func() (string, error) {
			return branch, nil
		},
	}

	cmd := NewCmdCheckout(factory, nil)

	argv, err := shlex.Split(cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(ioutil.Discard)
	cmd.SetErr(ioutil.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

func TestPRCheckout_sameRepo(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query PullRequestByNumber\b`), httpmock.StringResponse(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "REPO"
		},
		"isCrossRepository": false,
		"maintainerCanModify": false
	} } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/feature":
			return &errorStub{"exit status: 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := runCommand(http, nil, "master", `123`)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, "", output.String())
	if !assert.Equal(t, 4, len(ranCommands)) {
		return
	}
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin +refs/heads/feature:refs/remotes/origin/feature")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout -b feature --no-track origin/feature")
	eq(t, strings.Join(ranCommands[2], " "), "git config branch.feature.remote origin")
	eq(t, strings.Join(ranCommands[3], " "), "git config branch.feature.merge refs/heads/feature")
}

func TestPRCheckout_urlArg(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)
	http.Register(httpmock.GraphQL(`query PullRequestByNumber\b`), httpmock.StringResponse(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "REPO"
		},
		"isCrossRepository": false,
		"maintainerCanModify": false
	} } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/feature":
			return &errorStub{"exit status: 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := runCommand(http, nil, "master", `https://github.com/OWNER/REPO/pull/123/files`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 4)
	eq(t, strings.Join(ranCommands[1], " "), "git checkout -b feature --no-track origin/feature")
}

func TestPRCheckout_urlArg_differentBase(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)
	http.Register(httpmock.GraphQL(`query PullRequestByNumber\b`), httpmock.StringResponse(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "POE"
		},
		"isCrossRepository": false,
		"maintainerCanModify": false
	} } } }
	`))
	http.StubRepoInfoResponse("OWNER", "REPO", "master")

	ranCommands := [][]string{}
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/feature":
			return &errorStub{"exit status: 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := runCommand(http, nil, "master", `https://github.com/OTHER/POE/pull/123/files`)
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
	eq(t, strings.Join(ranCommands[1], " "), "git fetch https://github.com/OTHER/POE.git refs/pull/123/head:feature")
	eq(t, strings.Join(ranCommands[3], " "), "git config branch.feature.remote https://github.com/OTHER/POE.git")
}

func TestPRCheckout_branchArg(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query PullRequestForBranch\b`), httpmock.StringResponse(`
	{ "data": { "repository": { "pullRequests": { "nodes": [
		{ "number": 123,
		  "headRefName": "feature",
		  "headRepositoryOwner": {
		  	"login": "hubot"
		  },
		  "headRepository": {
		  	"name": "REPO"
		  },
		  "isCrossRepository": true,
		  "maintainerCanModify": false }
	] } } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/feature":
			return &errorStub{"exit status: 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := runCommand(http, nil, "master", `hubot:feature`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 5)
	eq(t, strings.Join(ranCommands[1], " "), "git fetch origin refs/pull/123/head:feature")
}

func TestPRCheckout_existingBranch(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query PullRequestByNumber\b`), httpmock.StringResponse(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "REPO"
		},
		"isCrossRepository": false,
		"maintainerCanModify": false
	} } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/feature":
			return &test.OutputStub{}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := runCommand(http, nil, "master", `123`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 3)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin +refs/heads/feature:refs/remotes/origin/feature")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout feature")
	eq(t, strings.Join(ranCommands[2], " "), "git merge --ff-only refs/remotes/origin/feature")
}

func TestPRCheckout_differentRepo_remoteExists(t *testing.T) {
	remotes := context.Remotes{
		{
			Remote: &git.Remote{Name: "origin"},
			Repo:   ghrepo.New("OWNER", "REPO"),
		},
		{
			Remote: &git.Remote{Name: "robot-fork"},
			Repo:   ghrepo.New("hubot", "REPO"),
		},
	}

	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query PullRequestByNumber\b`), httpmock.StringResponse(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "REPO"
		},
		"isCrossRepository": true,
		"maintainerCanModify": false
	} } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/feature":
			return &errorStub{"exit status: 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := runCommand(http, remotes, "master", `123`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 4)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch robot-fork +refs/heads/feature:refs/remotes/robot-fork/feature")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout -b feature --no-track robot-fork/feature")
	eq(t, strings.Join(ranCommands[2], " "), "git config branch.feature.remote robot-fork")
	eq(t, strings.Join(ranCommands[3], " "), "git config branch.feature.merge refs/heads/feature")
}

func TestPRCheckout_differentRepo(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query PullRequestByNumber\b`), httpmock.StringResponse(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "REPO"
		},
		"isCrossRepository": true,
		"maintainerCanModify": false
	} } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git config branch.feature.merge":
			return &errorStub{"exit status 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := runCommand(http, nil, "master", `123`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 4)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin refs/pull/123/head:feature")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout feature")
	eq(t, strings.Join(ranCommands[2], " "), "git config branch.feature.remote origin")
	eq(t, strings.Join(ranCommands[3], " "), "git config branch.feature.merge refs/pull/123/head")
}

func TestPRCheckout_differentRepo_existingBranch(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query PullRequestByNumber\b`), httpmock.StringResponse(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "REPO"
		},
		"isCrossRepository": true,
		"maintainerCanModify": false
	} } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git config branch.feature.merge":
			return &test.OutputStub{Out: []byte("refs/heads/feature\n")}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := runCommand(http, nil, "master", `123`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 2)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin refs/pull/123/head:feature")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout feature")
}

func TestPRCheckout_differentRepo_currentBranch(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query PullRequestByNumber\b`), httpmock.StringResponse(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "REPO"
		},
		"isCrossRepository": true,
		"maintainerCanModify": false
	} } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git config branch.feature.merge":
			return &test.OutputStub{Out: []byte("refs/heads/feature\n")}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := runCommand(http, nil, "feature", `123`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 2)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin refs/pull/123/head")
	eq(t, strings.Join(ranCommands[1], " "), "git merge --ff-only FETCH_HEAD")
}

func TestPRCheckout_differentRepo_invalidBranchName(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query PullRequestByNumber\b`), httpmock.StringResponse(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
		"headRefName": "-foo",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "REPO"
		},
		"isCrossRepository": true,
		"maintainerCanModify": false
	} } } }
	`))

	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		t.Errorf("unexpected external invocation: %v", cmd.Args)
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := runCommand(http, nil, "master", `123`)
	if assert.Errorf(t, err, "expected command to fail") {
		assert.Equal(t, `invalid branch name: "-foo"`, err.Error())
	}
	assert.Equal(t, "", output.Stderr())
}

func TestPRCheckout_maintainerCanModify(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query PullRequestByNumber\b`), httpmock.StringResponse(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "REPO"
		},
		"isCrossRepository": true,
		"maintainerCanModify": true
	} } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git config branch.feature.merge":
			return &errorStub{"exit status 1"}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := runCommand(http, nil, "master", `123`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 4)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin refs/pull/123/head:feature")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout feature")
	eq(t, strings.Join(ranCommands[2], " "), "git config branch.feature.remote https://github.com/hubot/REPO.git")
	eq(t, strings.Join(ranCommands[3], " "), "git config branch.feature.merge refs/heads/feature")
}

func TestPRCheckout_recurseSubmodules(t *testing.T) {
	http := &httpmock.Registry{}

	http.Register(httpmock.GraphQL(`query PullRequestByNumber\b`), httpmock.StringResponse(`
	{ "data": { "repository": { "pullRequest": {
		"number": 123,
		"headRefName": "feature",
		"headRepositoryOwner": {
			"login": "hubot"
		},
		"headRepository": {
			"name": "REPO"
		},
		"isCrossRepository": false,
		"maintainerCanModify": false
	} } } }
	`))

	ranCommands := [][]string{}
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case "git show-ref --verify -- refs/heads/feature":
			return &test.OutputStub{}
		default:
			ranCommands = append(ranCommands, cmd.Args)
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := runCommand(http, nil, "master", `123 --recurse-submodules`)
	eq(t, err, nil)
	eq(t, output.String(), "")

	eq(t, len(ranCommands), 5)
	eq(t, strings.Join(ranCommands[0], " "), "git fetch origin +refs/heads/feature:refs/remotes/origin/feature")
	eq(t, strings.Join(ranCommands[1], " "), "git checkout feature")
	eq(t, strings.Join(ranCommands[2], " "), "git merge --ff-only refs/remotes/origin/feature")
	eq(t, strings.Join(ranCommands[3], " "), "git submodule sync --recursive")
	eq(t, strings.Join(ranCommands[4], " "), "git submodule update --init --recursive")
}
