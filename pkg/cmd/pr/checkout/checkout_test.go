package checkout

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

// repo: either "baseOwner/baseRepo" or "baseOwner/baseRepo:defaultBranch"
// prHead: "headOwner/headRepo:headBranch"
func stubPR(repo, prHead string) (ghrepo.Interface, *api.PullRequest) {
	defaultBranch := ""
	if idx := strings.IndexRune(repo, ':'); idx >= 0 {
		defaultBranch = repo[idx+1:]
		repo = repo[:idx]
	}
	baseRepo, err := ghrepo.FromFullName(repo)
	if err != nil {
		panic(err)
	}
	if defaultBranch != "" {
		baseRepo = api.InitRepoHostname(&api.Repository{
			Name:             baseRepo.RepoName(),
			Owner:            api.RepositoryOwner{Login: baseRepo.RepoOwner()},
			DefaultBranchRef: api.BranchRef{Name: defaultBranch},
		}, baseRepo.RepoHost())
	}

	idx := strings.IndexRune(prHead, ':')
	headRefName := prHead[idx+1:]
	headRepo, err := ghrepo.FromFullName(prHead[:idx])
	if err != nil {
		panic(err)
	}

	return baseRepo, &api.PullRequest{
		Number:              123,
		HeadRefName:         headRefName,
		HeadRepositoryOwner: api.Owner{Login: headRepo.RepoOwner()},
		HeadRepository:      &api.PRRepository{Name: headRepo.RepoName()},
		IsCrossRepository:   !ghrepo.IsSame(baseRepo, headRepo),
		MaintainerCanModify: false,
	}
}

func Test_checkoutRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *CheckoutOptions
		httpStubs  func(*httpmock.Registry)
		runStubs   func(*run.CommandStubber)
		remotes    map[string]string
		wantStdout string
		wantStderr string
		wantErr    bool
	}{
		{
			name: "fork repo was deleted",
			opts: &CheckoutOptions{
				SelectorArg: "123",
				Finder: func() shared.PRFinder {
					baseRepo, pr := stubPR("OWNER/REPO:master", "hubot/REPO:feature")
					pr.MaintainerCanModify = true
					pr.HeadRepository = nil
					finder := shared.NewMockFinder("123", pr, baseRepo)
					return finder
				}(),
				Config: func() (config.Config, error) {
					return config.NewBlankConfig(), nil
				},
				Branch: func() (string, error) {
					return "main", nil
				},
			},
			remotes: map[string]string{
				"origin": "OWNER/REPO",
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git fetch origin refs/pull/123/head:feature`, 0, "")
				cs.Register(`git config branch\.feature\.merge`, 1, "")
				cs.Register(`git checkout feature`, 0, "")
				cs.Register(`git config branch\.feature\.remote origin`, 0, "")
				cs.Register(`git config branch\.feature\.pushRemote origin`, 0, "")
				cs.Register(`git config branch\.feature\.merge refs/pull/123/head`, 0, "")
			},
		},
		{
			name: "with local branch rename and existing git remote",
			opts: &CheckoutOptions{
				SelectorArg: "123",
				BranchName:  "foobar",
				Finder: func() shared.PRFinder {
					baseRepo, pr := stubPR("OWNER/REPO:master", "OWNER/REPO:feature")
					finder := shared.NewMockFinder("123", pr, baseRepo)
					return finder
				}(),
				Config: func() (config.Config, error) {
					return config.NewBlankConfig(), nil
				},
				Branch: func() (string, error) {
					return "main", nil
				},
			},
			remotes: map[string]string{
				"origin": "OWNER/REPO",
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git show-ref --verify -- refs/heads/foobar`, 1, "")
				cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature`, 0, "")
				cs.Register(`git checkout -b foobar --track origin/feature`, 0, "")
			},
		},
		{
			name: "with local branch name, no existing git remote",
			opts: &CheckoutOptions{
				SelectorArg: "123",
				BranchName:  "foobar",
				Finder: func() shared.PRFinder {
					baseRepo, pr := stubPR("OWNER/REPO:master", "hubot/REPO:feature")
					pr.MaintainerCanModify = true
					finder := shared.NewMockFinder("123", pr, baseRepo)
					return finder
				}(),
				Config: func() (config.Config, error) {
					return config.NewBlankConfig(), nil
				},
				Branch: func() (string, error) {
					return "main", nil
				},
			},
			remotes: map[string]string{
				"origin": "OWNER/REPO",
			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config branch\.foobar\.merge`, 1, "")
				cs.Register(`git fetch origin refs/pull/123/head:foobar`, 0, "")
				cs.Register(`git checkout foobar`, 0, "")
				cs.Register(`git config branch\.foobar\.remote https://github.com/hubot/REPO.git`, 0, "")
				cs.Register(`git config branch\.foobar\.pushRemote https://github.com/hubot/REPO.git`, 0, "")
				cs.Register(`git config branch\.foobar\.merge refs/heads/feature`, 0, "")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts

			ios, _, stdout, stderr := iostreams.Test()
			opts.IO = ios
			httpReg := &httpmock.Registry{}
			defer httpReg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(httpReg)
			}
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: httpReg}, nil
			}

			cmdStubs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)
			if tt.runStubs != nil {
				tt.runStubs(cmdStubs)
			}

			opts.Remotes = func() (context.Remotes, error) {
				if len(tt.remotes) == 0 {
					return nil, errors.New("no remotes")
				}
				var remotes context.Remotes
				for name, repo := range tt.remotes {
					r, err := ghrepo.FromFullName(repo)
					if err != nil {
						return remotes, err
					}
					remotes = append(remotes, &context.Remote{
						Remote: &git.Remote{Name: name},
						Repo:   r,
					})
				}
				return remotes, nil
			}

			opts.GitClient = &git.Client{
				GhPath:  "some/path/gh",
				GitPath: "some/path/git",
			}

			err := checkoutRun(opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("want error: %v, got: %v", tt.wantErr, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

/** LEGACY TESTS **/

func runCommand(rt http.RoundTripper, remotes context.Remotes, branch string, cli string) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()

	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
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
		GitClient: &git.Client{
			GhPath:  "some/path/gh",
			GitPath: "some/path/git",
		},
	}

	cmd := NewCmdCheckout(factory, nil)

	argv, err := shlex.Split(cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

func TestPRCheckout_sameRepo(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO", "OWNER/REPO:feature")
	finder := shared.RunCommandFinder("123", pr, baseRepo)
	finder.ExpectFields([]string{"number", "headRefName", "headRepository", "headRepositoryOwner", "isCrossRepository", "maintainerCanModify"})

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 1, "")
	cs.Register(`git checkout -b feature --track origin/feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRCheckout_existingBranch(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO", "OWNER/REPO:feature")
	shared.RunCommandFinder("123", pr, baseRepo)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 0, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git merge --ff-only refs/remotes/origin/feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
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

	baseRepo, pr := stubPR("OWNER/REPO", "hubot/REPO:feature")
	finder := shared.RunCommandFinder("123", pr, baseRepo)
	finder.ExpectFields([]string{"number", "headRefName", "headRepository", "headRepositoryOwner", "isCrossRepository", "maintainerCanModify"})

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch robot-fork \+refs/heads/feature:refs/remotes/robot-fork/feature`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 1, "")
	cs.Register(`git checkout -b feature --track robot-fork/feature`, 0, "")

	output, err := runCommand(http, remotes, "master", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRCheckout_differentRepo(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO:master", "hubot/REPO:feature")
	finder := shared.RunCommandFinder("123", pr, baseRepo)
	finder.ExpectFields([]string{"number", "headRefName", "headRepository", "headRepositoryOwner", "isCrossRepository", "maintainerCanModify"})

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin refs/pull/123/head:feature`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 1, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git config branch\.feature\.remote origin`, 0, "")
	cs.Register(`git config branch\.feature\.pushRemote origin`, 0, "")
	cs.Register(`git config branch\.feature\.merge refs/pull/123/head`, 0, "")

	output, err := runCommand(http, nil, "master", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRCheckout_differentRepo_existingBranch(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO:master", "hubot/REPO:feature")
	shared.RunCommandFinder("123", pr, baseRepo)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin refs/pull/123/head:feature`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 0, "refs/heads/feature\n")
	cs.Register(`git checkout feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRCheckout_detachedHead(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO:master", "hubot/REPO:feature")
	shared.RunCommandFinder("123", pr, baseRepo)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin refs/pull/123/head:feature`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 0, "refs/heads/feature\n")
	cs.Register(`git checkout feature`, 0, "")

	output, err := runCommand(http, nil, "", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRCheckout_differentRepo_currentBranch(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO:master", "hubot/REPO:feature")
	shared.RunCommandFinder("123", pr, baseRepo)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin refs/pull/123/head`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 0, "refs/heads/feature\n")
	cs.Register(`git merge --ff-only FETCH_HEAD`, 0, "")

	output, err := runCommand(http, nil, "feature", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRCheckout_differentRepo_invalidBranchName(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO", "hubot/REPO:-foo")
	shared.RunCommandFinder("123", pr, baseRepo)

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	output, err := runCommand(http, nil, "master", `123`)
	assert.EqualError(t, err, `invalid branch name: "-foo"`)
	assert.Equal(t, "", output.Stderr())
	assert.Equal(t, "", output.Stderr())
}

func TestPRCheckout_maintainerCanModify(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO:master", "hubot/REPO:feature")
	pr.MaintainerCanModify = true
	shared.RunCommandFinder("123", pr, baseRepo)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin refs/pull/123/head:feature`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 1, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git config branch\.feature\.remote https://github\.com/hubot/REPO\.git`, 0, "")
	cs.Register(`git config branch\.feature\.pushRemote https://github\.com/hubot/REPO\.git`, 0, "")
	cs.Register(`git config branch\.feature\.merge refs/heads/feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRCheckout_recurseSubmodules(t *testing.T) {
	http := &httpmock.Registry{}

	baseRepo, pr := stubPR("OWNER/REPO", "OWNER/REPO:feature")
	shared.RunCommandFinder("123", pr, baseRepo)

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
	assert.Equal(t, "", output.Stderr())
}

func TestPRCheckout_force(t *testing.T) {
	http := &httpmock.Registry{}

	baseRepo, pr := stubPR("OWNER/REPO", "OWNER/REPO:feature")
	shared.RunCommandFinder("123", pr, baseRepo)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 0, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git reset --hard refs/remotes/origin/feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123 --force`)

	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRCheckout_detach(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO:master", "hubot/REPO:feature")
	shared.RunCommandFinder("123", pr, baseRepo)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git checkout --detach FETCH_HEAD`, 0, "")
	cs.Register(`git fetch origin refs/pull/123/head`, 0, "")

	output, err := runCommand(http, nil, "", `123 --detach`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}
