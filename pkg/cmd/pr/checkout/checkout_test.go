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
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdCheckout(t *testing.T) {
	tests := []struct {
		name      string
		args      string
		wantsOpts CheckoutOptions
		wantErr   error
	}{
		{
			name: "recurse submodules",
			args: "--recurse-submodules 123",
			wantsOpts: CheckoutOptions{
				RecurseSubmodules: true,
			},
		},
		{
			name: "force",
			args: "--force 123",
			wantsOpts: CheckoutOptions{
				Force: true,
			},
		},
		{
			name: "detach",
			args: "--detach 123",
			wantsOpts: CheckoutOptions{
				Detach: true,
			},
		},
		{
			name: "branch",
			args: "--branch test-branch 123",
			wantsOpts: CheckoutOptions{
				BranchName: "test-branch",
			},
		},
		{
			name:    "when there is no selector and no TTY, returns an error",
			args:    "",
			wantErr: cmdutil.FlagErrorf("pull request number, URL, or branch required when not running interactively"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			ios.SetStdinTTY(false)

			argv, err := shlex.Split(tt.args)
			assert.NoError(t, err)

			var spiedOpts *CheckoutOptions
			cmd := NewCmdCheckout(f, func(opts *CheckoutOptions) error {
				spiedOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantsOpts.RecurseSubmodules, spiedOpts.RecurseSubmodules)
			require.Equal(t, tt.wantsOpts.Force, spiedOpts.Force)
			require.Equal(t, tt.wantsOpts.Detach, spiedOpts.Detach)
			require.Equal(t, tt.wantsOpts.BranchName, spiedOpts.BranchName)
		})
	}
}

// repo: either "baseOwner/baseRepo" or "baseOwner/baseRepo:defaultBranch"
// prHead: "headOwner/headRepo:headBranch"
func stubPR(repo, prHead string) (ghrepo.Interface, *api.PullRequest) {
	return _stubPR(repo, prHead, 123, "PR title", "OPEN", false)
}

func _stubPR(repo, prHead string, number int, title string, state string, isDraft bool) (ghrepo.Interface, *api.PullRequest) {
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
		Number:              number,
		HeadRefName:         headRefName,
		HeadRepositoryOwner: api.Owner{Login: headRepo.RepoOwner()},
		HeadRepository:      &api.PRRepository{Name: headRepo.RepoName()},
		IsCrossRepository:   !ghrepo.IsSame(baseRepo, headRepo),
		MaintainerCanModify: false,

		Title:   title,
		State:   state,
		IsDraft: isDraft,
	}
}

type stubPRResolver struct {
	pr       *api.PullRequest
	baseRepo ghrepo.Interface

	err error
}

func (s *stubPRResolver) Resolve() (*api.PullRequest, ghrepo.Interface, error) {
	if s.err != nil {
		return nil, nil, s.err
	}
	return s.pr, s.baseRepo, nil
}

func Test_checkoutRun(t *testing.T) {
	tests := []struct {
		name string
		opts *CheckoutOptions

		httpStubs   func(*httpmock.Registry)
		runStubs    func(*run.CommandStubber)
		promptStubs func(*prompter.MockPrompter)

		remotes    map[string]string
		wantStdout string
		wantStderr string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "checkout with ssh remote URL",
			opts: &CheckoutOptions{
				PRResolver: func() PRResolver {
					baseRepo, pr := stubPR("OWNER/REPO:master", "OWNER/REPO:feature")
					return &stubPRResolver{
						pr:       pr,
						baseRepo: baseRepo,
					}
				}(),
				Config: func() (gh.Config, error) {
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
				cs.Register(`git show-ref --verify -- refs/heads/feature`, 1, "")
				cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature --no-tags`, 0, "")
				cs.Register(`git checkout -b feature --track origin/feature`, 0, "")
			},
		},
		{
			name: "fork repo was deleted",
			opts: &CheckoutOptions{
				PRResolver: func() PRResolver {
					baseRepo, pr := stubPR("OWNER/REPO:master", "OWNER/REPO:feature")
					pr.MaintainerCanModify = true
					pr.HeadRepository = nil
					return &stubPRResolver{
						pr:       pr,
						baseRepo: baseRepo,
					}
				}(),
				Config: func() (gh.Config, error) {
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
				cs.Register(`git fetch origin refs/pull/123/head:feature --no-tags`, 0, "")
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
				BranchName: "foobar",
				PRResolver: func() PRResolver {
					baseRepo, pr := stubPR("OWNER/REPO:master", "OWNER/REPO:feature")
					return &stubPRResolver{
						pr:       pr,
						baseRepo: baseRepo,
					}
				}(),
				Config: func() (gh.Config, error) {
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
				cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature --no-tags`, 0, "")
				cs.Register(`git checkout -b foobar --track origin/feature`, 0, "")
			},
		},
		{
			name: "with local branch name, no existing git remote",
			opts: &CheckoutOptions{
				BranchName: "foobar",
				PRResolver: func() PRResolver {
					baseRepo, pr := stubPR("OWNER/REPO:master", "hubot/REPO:feature")
					pr.MaintainerCanModify = true
					return &stubPRResolver{
						pr:       pr,
						baseRepo: baseRepo,
					}
				}(),
				Config: func() (gh.Config, error) {
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
				cs.Register(`git fetch origin refs/pull/123/head:foobar --no-tags`, 0, "")
				cs.Register(`git checkout foobar`, 0, "")
				cs.Register(`git config branch\.foobar\.remote https://github.com/hubot/REPO.git`, 0, "")
				cs.Register(`git config branch\.foobar\.pushRemote https://github.com/hubot/REPO.git`, 0, "")
				cs.Register(`git config branch\.foobar\.merge refs/heads/feature`, 0, "")
			},
		},
		{
			name: "when the PR resolver errors, then that error is bubbled up",
			opts: &CheckoutOptions{
				PRResolver: &stubPRResolver{
					err: errors.New("expected test error"),
				},
			},
			wantErr: true,
			errMsg:  "expected test error",
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
			if err != nil {
				assert.Equal(t, tt.errMsg, err.Error())
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

func TestSpecificPRResolver(t *testing.T) {
	t.Run("when the PR Finder returns results, those are returned", func(t *testing.T) {
		t.Parallel()

		baseRepo, pr := stubPR("OWNER/REPO:master", "OWNER/REPO:feature")
		mockFinder := shared.NewMockFinder("123", pr, baseRepo)
		mockFinder.ExpectFields([]string{"number", "headRefName", "headRepository", "headRepositoryOwner", "isCrossRepository", "maintainerCanModify"})

		resolver := &specificPRResolver{
			prFinder: mockFinder,
			selector: "123",
		}

		resolvedPR, resolvedBaseRepo, err := resolver.Resolve()
		require.NoError(t, err)
		require.Equal(t, pr, resolvedPR)
		require.True(t, ghrepo.IsSame(baseRepo, resolvedBaseRepo), "expected repos to be the same")
	})

	t.Run("when the PR Finder errors, that error is returned", func(t *testing.T) {
		t.Parallel()

		mockFinder := shared.NewMockFinder("123", nil, nil)

		resolver := &specificPRResolver{
			prFinder: mockFinder,
			selector: "123",
		}

		_, _, err := resolver.Resolve()
		var notFoundErr *shared.NotFoundError
		require.ErrorAs(t, err, &notFoundErr)
	})
}

func TestPromptingPRResolver(t *testing.T) {
	t.Run("when the PR Lister has results, then we prompt for a choice", func(t *testing.T) {
		t.Parallel()

		ios, _, _, _ := iostreams.Test()

		baseRepo, pr1 := _stubPR("OWNER/REPO:master", "OWNER/REPO:feature", 32, "New feature", "OPEN", false)
		_, pr2 := _stubPR("OWNER/REPO:master", "OWNER/REPO:bug-fix", 29, "Fixed bad bug", "OPEN", false)
		_, pr3 := _stubPR("OWNER/REPO:master", "OWNER/REPO:docs", 28, "Improve documentation", "OPEN", true)
		lister := shared.NewMockLister(&api.PullRequestAndTotalCount{
			TotalCount: 3,
			PullRequests: []api.PullRequest{
				*pr1, *pr2, *pr3,
			}, SearchCapped: false}, nil)
		lister.ExpectFields([]string{"number", "title", "state", "isDraft", "headRefName", "headRepository", "headRepositoryOwner", "isCrossRepository", "maintainerCanModify"})

		pm := prompter.NewMockPrompter(t)
		pm.RegisterSelect("Select a pull request",
			[]string{"32\tOPEN New feature [feature]", "29\tOPEN Fixed bad bug [bug-fix]", "28\tDRAFT Improve documentation [docs]"},
			func(_, _ string, opts []string) (int, error) {
				return prompter.IndexFor(opts, "32\tOPEN New feature [feature]")
			})

		resolver := &promptingPRResolver{
			io:       ios,
			prompter: pm,

			prLister: lister,

			baseRepo: baseRepo,
		}

		resolvedPR, resolvedBaseRepo, err := resolver.Resolve()
		require.NoError(t, err)
		require.Equal(t, pr1, resolvedPR)
		require.True(t, ghrepo.IsSame(baseRepo, resolvedBaseRepo), "expected repos to be the same")
	})

	t.Run("when the PR lister has no results, then we return an error", func(t *testing.T) {
		t.Parallel()

		ios, _, _, _ := iostreams.Test()

		lister := shared.NewMockLister(&api.PullRequestAndTotalCount{
			TotalCount:   0,
			PullRequests: []api.PullRequest{},
		}, nil)

		resolver := &promptingPRResolver{
			io:       ios,
			prLister: lister,
			baseRepo: ghrepo.New("OWNER", "REPO"),
		}

		_, _, err := resolver.Resolve()
		var noResultsErr cmdutil.NoResultsError
		require.ErrorAs(t, err, &noResultsErr)
		require.Equal(t, "no open pull requests in OWNER/REPO", noResultsErr.Error())
	})
}

/** LEGACY TESTS **/

func runCommand(rt http.RoundTripper, remotes context.Remotes, branch string, cli string, baseRepo ghrepo.Interface) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()

	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (gh.Config, error) {
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
		BaseRepo: func() (ghrepo.Interface, error) {
			return baseRepo, nil
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

	cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature --no-tags`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 1, "")
	cs.Register(`git checkout -b feature --track origin/feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123`, baseRepo)
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
	cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature --no-tags`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 0, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git merge --ff-only refs/remotes/origin/feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123`, baseRepo)
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
	cs.Register(`git fetch robot-fork \+refs/heads/feature:refs/remotes/robot-fork/feature --no-tags`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 1, "")
	cs.Register(`git checkout -b feature --track robot-fork/feature`, 0, "")

	output, err := runCommand(http, remotes, "master", `123`, baseRepo)
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
	cs.Register(`git fetch origin refs/pull/123/head:feature --no-tags`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 1, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git config branch\.feature\.remote origin`, 0, "")
	cs.Register(`git config branch\.feature\.pushRemote origin`, 0, "")
	cs.Register(`git config branch\.feature\.merge refs/pull/123/head`, 0, "")

	output, err := runCommand(http, nil, "master", `123`, baseRepo)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRCheckout_differentRepoForce(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO:master", "hubot/REPO:feature")
	finder := shared.RunCommandFinder("123", pr, baseRepo)
	finder.ExpectFields([]string{"number", "headRefName", "headRepository", "headRepositoryOwner", "isCrossRepository", "maintainerCanModify"})

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	cs.Register(`git fetch origin refs/pull/123/head:feature --no-tags --force`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 1, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git config branch\.feature\.remote origin`, 0, "")
	cs.Register(`git config branch\.feature\.pushRemote origin`, 0, "")
	cs.Register(`git config branch\.feature\.merge refs/pull/123/head`, 0, "")

	output, err := runCommand(http, nil, "master", `123 --force`, baseRepo)
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
	cs.Register(`git fetch origin refs/pull/123/head:feature --no-tags`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 0, "refs/heads/feature\n")
	cs.Register(`git checkout feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123`, baseRepo)
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
	cs.Register(`git fetch origin refs/pull/123/head:feature --no-tags`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 0, "refs/heads/feature\n")
	cs.Register(`git checkout feature`, 0, "")

	output, err := runCommand(http, nil, "", `123`, baseRepo)
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
	cs.Register(`git fetch origin refs/pull/123/head --no-tags`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 0, "refs/heads/feature\n")
	cs.Register(`git merge --ff-only FETCH_HEAD`, 0, "")

	output, err := runCommand(http, nil, "feature", `123`, baseRepo)
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

	output, err := runCommand(http, nil, "master", `123`, baseRepo)

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
	cs.Register(`git fetch origin refs/pull/123/head:feature --no-tags`, 0, "")
	cs.Register(`git config branch\.feature\.merge`, 1, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git config branch\.feature\.remote https://github\.com/hubot/REPO\.git`, 0, "")
	cs.Register(`git config branch\.feature\.pushRemote https://github\.com/hubot/REPO\.git`, 0, "")
	cs.Register(`git config branch\.feature\.merge refs/heads/feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123`, baseRepo)
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
	cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature --no-tags`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 0, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git merge --ff-only refs/remotes/origin/feature`, 0, "")
	cs.Register(`git submodule sync --recursive`, 0, "")
	cs.Register(`git submodule update --init --recursive`, 0, "")

	output, err := runCommand(http, nil, "master", `123 --recurse-submodules`, baseRepo)
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
	cs.Register(`git fetch origin \+refs/heads/feature:refs/remotes/origin/feature --no-tags`, 0, "")
	cs.Register(`git show-ref --verify -- refs/heads/feature`, 0, "")
	cs.Register(`git checkout feature`, 0, "")
	cs.Register(`git reset --hard refs/remotes/origin/feature`, 0, "")

	output, err := runCommand(http, nil, "master", `123 --force`, baseRepo)

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
	cs.Register(`git fetch origin refs/pull/123/head --no-tags`, 0, "")

	output, err := runCommand(http, nil, "", `123 --detach`, baseRepo)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}
