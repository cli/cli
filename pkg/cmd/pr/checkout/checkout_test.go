package checkout

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 1, "")
	cs.Register(`git checkout -b feature --no-track origin/feature`, 0, "")
	cs.Register(`git config branch\.feature\.remote origin`, 0, "")
	cs.Register(`git config branch\.feature\.merge refs/heads/feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123`)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, "", output.String())
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 1, "")
	cs.Register(`git checkout -b feature --no-track origin/feature`, 0, "")
	cs.Register(`git config branch\.feature\.remote origin`, 0, "")
	cs.Register(`git config branch\.feature\.merge refs/heads/feature`, 0, "")

	output, err := runCommand(http, nil, "master", `https://github.com/OWNER/REPO/pull/123/files`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch https://github\.com/OTHER/POE\.git refs/pull/123/head:feature`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 1, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git config branch\.feature\.remote https://github\.com/OTHER/POE\.git`, 0, "")
	cs.Register(`git config branch\.feature\.merge refs/pull/123/head`, 0, "")

	output, err := runCommand(http, nil, "master", `https://github.com/OTHER/POE/pull/123/files`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	reqBody := struct {
		Variables struct {
			Owner string
			Repo  string
		}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	assert.Equal(t, "OTHER", reqBody.Variables.Owner)
	assert.Equal(t, "POE", reqBody.Variables.Repo)
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin refs/pull/123/head:feature`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 1, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git config branch\.feature\.remote origin`, 0, "")
	cs.Register(`git config branch\.feature\.merge refs/pull/123/head`, 0, "")

	output, err := runCommand(http, nil, "master", `hubot:feature`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 0, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git merge --ff-only refs/remotes/origin/feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch robot-fork \+refs/heads/feature:refs/remotes/robot-fork/feature`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 1, "")
	cs.Register(`git checkout -b feature --no-track robot-fork/feature`, 0, "")
	cs.Register(`git config branch\.feature\.remote robot-fork`, 0, "")
	cs.Register(`git config branch\.feature\.merge refs/heads/feature`, 0, "")

	output, err := runCommand(http, remotes, "master", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin refs/pull/123/head:feature`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 1, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git config branch\.feature\.remote origin`, 0, "")
	cs.Register(`git config branch\.feature\.merge refs/pull/123/head`, 0, "")

	output, err := runCommand(http, nil, "master", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin refs/pull/123/head:feature`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 0, "refs/heads/feature\n")
	cs.Register(`git checkout feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
}

func TestPRCheckout_detachedHead(t *testing.T) {
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin refs/pull/123/head:feature`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 0, "refs/heads/feature\n")
	cs.Register(`git checkout feature`, 0, "")

	output, err := runCommand(http, nil, "", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin refs/pull/123/head`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 0, "refs/heads/feature\n")
	cs.Register(`git merge --ff-only FETCH_HEAD`, 0, "")

	output, err := runCommand(http, nil, "feature", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
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

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	output, err := runCommand(http, nil, "master", `123`)
	assert.EqualError(t, err, `invalid branch name: "-foo"`)
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin refs/pull/123/head:feature`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 1, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git config branch\.feature\.remote https://github\.com/hubot/REPO\.git`, 0, "")
	cs.Register(`git config branch\.feature\.merge refs/heads/feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 0, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git merge --ff-only refs/remotes/origin/feature`, 0, "")
	cs.Register(`git submodule sync --recursive`, 0, "")
	cs.Register(`git submodule update --init --recursive`, 0, "")

	output, err := runCommand(http, nil, "master", `123 --recurse-submodules`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
}

func TestPRCheckout_force(t *testing.T) {
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 0, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git reset --hard refs/remotes/origin/feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123 --force`)

	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
}

func TestPRCheckout_detach(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query PullRequestByNumber\b`), httpmock.StringResponse(`
 	{ "data": { "repository": { "pullRequest": {
 		"number": 123,
 		"headRef": "f8f8f8",
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git checkout --detach FETCH_HEAD`, 0, "")
	cs.Register(`git fetch origin refs/pull/123/head`, 0, "")

	output, err := runCommand(http, nil, "", `123 --detach`)
	assert.Nil(t, err)
	assert.Equal(t, "", output.String())
}
