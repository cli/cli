package merge

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
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

const (
	// The reporter's setup: a pull request in OWNER-A/REPO-A whose head branch
	// happens to be called "feature", and a current working directory inside a
	// different repository that also has a local branch of that name.
	cleanupPRURL      = "https://github.com/OWNER-A/REPO-A/pull/7"
	cleanupPROwner    = "OWNER-A"
	cleanupPRName     = "REPO-A"
	cleanupPRBranch   = "OWNER-A/REPO-A"
	cleanupHeadRef    = "feature"
	cleanupHeadOID    = "1111111111111111111111111111111111111111"
	cleanupLocalTip   = "2222222222222222222222222222222222222222"
	cleanupOtherRepo  = "OWNER-B/REPO-B"
	cleanupForkRepo   = "MONALISA/REPO-A"
	cleanupRemoteName = "origin"
)

// remotesFor builds the GitHub remotes of the repository gh is running in. The
// first one is named origin, which is the remote the base branch is pulled from
// when cleanup switches the current worktree over to it.
func remotesFor(fullNames ...string) context.Remotes {
	names := []string{"origin", "upstream"}
	remotes := context.Remotes{}
	for i, fullName := range fullNames {
		repo, err := ghrepo.FromFullName(fullName)
		if err != nil {
			panic(err)
		}
		name := fmt.Sprintf("remote-%d", i)
		if i < len(names) {
			name = names[i]
		}
		remotes = append(remotes, &context.Remote{
			Remote: &git.Remote{Name: name},
			Repo:   repo,
		})
	}
	return remotes
}

// worktreeList is `git worktree list --porcelain` output for a main worktree on
// main, with the pull request's head branch optionally checked out in a linked
// worktree.
func worktreeList(headBranchInWorktree bool) string {
	if !headBranchInWorktree {
		return heredoc.Doc(`
			worktree /path/to/main
			HEAD abc123
			branch refs/heads/main
		`)
	}
	return heredoc.Doc(`
		worktree /path/to/main
		HEAD abc123
		branch refs/heads/main

		worktree /path/to/feature-wt
		HEAD def456
		branch refs/heads/feature
	`)
}

// destructiveRan returns the recorded git commands that would destroy local
// state. An empty result is what the "leave the local repository alone" tests
// are asserting on.
func destructiveRan(ran []string) []string {
	destructive := []string{}
	for _, line := range ran {
		if strings.Contains(line, "branch -D") || strings.Contains(line, "worktree remove") {
			destructive = append(destructive, line)
		}
	}
	return destructive
}

func ranSomething(ran []string, substring string) bool {
	return slices.ContainsFunc(ran, func(line string) bool {
		return strings.Contains(line, substring)
	})
}

// cleanupScenario describes the state of the local repository and of the pull
// request for one run of `gh pr merge <PR-URL> --merge --delete-branch`.
type cleanupScenario struct {
	// remotes of the repository gh is running in, and the error to return from
	// reading them instead
	remotes    context.Remotes
	remotesErr error
	// headRefOid of the pull request; empty leaves it unset
	headRefOid string
	// whether the local branch of the same name holds commits that are not on
	// the pull request head
	localTipAhead bool
	// whether the head branch is checked out in a linked worktree
	headBranchInWorktree bool
	// whether the pull request is from a fork
	crossRepository bool
}

// runCleanupScenario runs the merge command end to end against stubbed git and
// returns its output together with every git command that was run.
//
// The teardown of run.Stub is deliberately dropped: a stub left unmatched means
// a command was not run, which is precisely what these tests assert on, and the
// teardown would report that as an error. A catch-all stub records everything
// else so that no unexpected git command can slip past unnoticed either.
func runCleanupScenario(t *testing.T, tt cleanupScenario) (*test.CmdOut, []string, error) {
	t.Helper()

	reg := initFakeHTTP()
	defer reg.Verify(t)

	pr := &api.PullRequest{
		ID:               "THE-ID",
		Number:           7,
		State:            "OPEN",
		Title:            "The title of the PR",
		HeadRefName:      cleanupHeadRef,
		HeadRefOid:       tt.headRefOid,
		MergeStateStatus: "CLEAN",
		BaseRefName:      "main",
	}
	if tt.crossRepository {
		pr.IsCrossRepository = true
		pr.HeadRepositoryOwner = api.Owner{Login: "MONALISA"}
		pr.HeadRepository = &api.PRRepository{Name: "REPO-A"}
	}

	shared.StubFinderForRunCommandStyleTests(t, cleanupPRURL, pr, baseRepo(cleanupPROwner, cleanupPRName, "main"))

	reg.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]any) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
		}))
	// Deleting the remote branch short-circuits for a cross-repository pull
	// request, so that request is only expected for a same-repository one.
	if !tt.crossRepository {
		reg.Register(
			httpmock.REST("DELETE", "repos/OWNER-A/REPO-A/git/refs/heads%2Ffeature"),
			httpmock.StringResponse(`{}`))
	}

	cs, _ := run.Stub()
	ran := []string{}
	record := func(args []string) {
		ran = append(ran, strings.Join(args, " "))
	}

	localTip := cleanupHeadOID
	if tt.localTipAhead {
		localTip = cleanupLocalTip
	}
	// Registered twice because the branch is both looked up and, when it is in
	// scope, read again for its tip.
	cs.Register(`git rev-parse --verify refs/heads/feature`, 0, localTip+"\n")
	cs.Register(`git rev-parse --verify refs/heads/feature`, 0, localTip+"\n")
	cs.Register(`git worktree list --porcelain`, 0, worktreeList(tt.headBranchInWorktree))
	cs.Register(`git rev-parse --show-toplevel`, 0, "/path/to/main")
	// `git merge-base --is-ancestor` is silent and reports through its exit
	// status: 0 when the local tip is an ancestor of, or equal to, the pull
	// request head, and 1 when it is not.
	ancestorExit := 1
	if !tt.localTipAhead {
		ancestorExit = 0
	}
	cs.Register(`git merge-base --is-ancestor \S+ \S+`, ancestorExit, "")
	cs.Register(`git rev-parse --verify refs/heads/main`, 0, "abc123\n")
	cs.Register(`git checkout main`, 0, "")
	cs.Register(`git .*pull --ff-only origin main`, 0, "")
	cs.Register(`git worktree remove -- /path/to/feature-wt`, 0, "", record)
	cs.Register(`git branch -D feature`, 0, "", record)
	cs.Register(`git .*`, 0, "", record)

	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStdinTTY(true)
	ios.SetStderrTTY(true)

	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		Branch: func() (string, error) {
			return "main", nil
		},
		Remotes: func() (context.Remotes, error) {
			if tt.remotesErr != nil {
				return nil, tt.remotesErr
			}
			return tt.remotes, nil
		},
		GitClient: &git.Client{
			GhPath:  "some/path/gh",
			GitPath: "some/path/git",
		},
	}

	cmd := NewCmdMerge(factory, nil)
	cmd.PersistentFlags().StringP("repo", "R", "", "")

	argv, err := shlex.Split(cleanupPRURL + " --merge --delete-branch")
	require.NoError(t, err)
	cmd.SetArgs(argv)
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{OutBuf: stdout, ErrBuf: stderr}, ran, err
}

func TestPrMerge_deleteBranch_sameNameInAnotherRepo(t *testing.T) {
	// Given a pull request in OWNER-A/REPO-A, and a current directory whose
	// only remote points at OWNER-B/REPO-B, which has a local branch called
	// "feature" of its own.
	//
	// When the pull request is merged by URL with --delete-branch.
	//
	// Then OWNER-B/REPO-B is not the pull request's repository, so its branch
	// is left alone while the remote branch of the pull request still goes.
	output, ran, err := runCleanupScenario(t, cleanupScenario{
		remotes:    remotesFor(cleanupOtherRepo),
		headRefOid: cleanupHeadOID,
	})
	require.NoError(t, err)

	assert.Empty(t, destructiveRan(ran))
	assert.Contains(t, output.Stderr(), "Skipping local branch delete")
	assert.Contains(t, output.Stderr(), "is not a clone of the base or head repository of pull request OWNER-A/REPO-A#7")
	assert.NotContains(t, output.Stderr(), "Deleted local branch")
	assert.Contains(t, output.Stderr(), "Deleted remote branch feature")
}

func TestPrMerge_deleteBranch_sameNameInAnotherRepoInWorktree(t *testing.T) {
	// The reporter's worst case: the unrelated repository has the same-named
	// branch checked out in a linked worktree, so both the worktree removal and
	// the force delete have to be withheld, since the worktree is where the
	// unpushed commits live.
	output, ran, err := runCleanupScenario(t, cleanupScenario{
		remotes:              remotesFor(cleanupOtherRepo),
		headRefOid:           cleanupHeadOID,
		headBranchInWorktree: true,
	})
	require.NoError(t, err)

	assert.Empty(t, destructiveRan(ran))
	assert.NotContains(t, output.Stderr(), "Removed worktree")
	assert.NotContains(t, output.Stderr(), "Deleted local branch")
	assert.Contains(t, output.Stderr(), "is not a clone of the base or head repository")
}

func TestPrMerge_deleteBranch_sameNameInAnotherRepoDifferentCase(t *testing.T) {
	// Owner and name casing differs between the remote and the pull request.
	// Comparison is by owner and name, case-insensitively, so an unrelated
	// repository that merely looks similar is still out of scope.
	output, ran, err := runCleanupScenario(t, cleanupScenario{
		remotes:    remotesFor("owner-b/repo-b"),
		headRefOid: cleanupHeadOID,
	})
	require.NoError(t, err)

	assert.Empty(t, destructiveRan(ran))
	assert.Contains(t, output.Stderr(), "is not a clone of the base or head repository")
	assert.NotContains(t, output.Stderr(), "Deleted local branch")
}

func TestPrMerge_deleteBranch_noGitHubRemotes(t *testing.T) {
	// A directory with no GitHub remotes is not the pull request's repository
	// either, and is treated as such rather than as a match.
	output, ran, err := runCleanupScenario(t, cleanupScenario{
		remotes:    context.Remotes{},
		headRefOid: cleanupHeadOID,
	})
	require.NoError(t, err)

	assert.Empty(t, destructiveRan(ran))
	assert.Contains(t, output.Stderr(), "is not a clone of the base or head repository")
	assert.NotContains(t, output.Stderr(), "Deleted local branch")
}

func TestPrMerge_deleteBranch_remotesFail(t *testing.T) {
	// When the remotes cannot be read, the scope of the pull request is unknown,
	// so cleanup is skipped rather than assumed safe.
	output, ran, err := runCleanupScenario(t, cleanupScenario{
		remotesErr:           errors.New("failed to read remotes"),
		headRefOid:           cleanupHeadOID,
		headBranchInWorktree: true,
	})
	require.NoError(t, err)

	assert.Empty(t, destructiveRan(ran))
	assert.Contains(t, output.Stderr(), "Could not determine the repositories of the current directory; skipping local branch delete: failed to read remotes")
	assert.NotContains(t, output.Stderr(), "Deleted local branch")
}

func TestPrMerge_deleteBranch_localTipNotInPR(t *testing.T) {
	// Given the pull request's own repository, and a local "feature" branch
	// whose tip has commits that are not on the pull request head.
	//
	// When it is merged with --delete-branch.
	//
	// Then the branch survives and the user is told how to inspect it and delete
	// it by hand. `git branch -D` skips the merged-check that would otherwise
	// have caught this.
	output, ran, err := runCleanupScenario(t, cleanupScenario{
		remotes:       remotesFor(cleanupPRBranch),
		headRefOid:    cleanupHeadOID,
		localTipAhead: true,
	})
	require.NoError(t, err)

	assert.Empty(t, destructiveRan(ran))
	assert.Contains(t, output.Stderr(), "has commits that are not in pull request OWNER-A/REPO-A#7; skipping local branch delete")
	assert.Contains(t, output.Stderr(), "To see what would be lost, and to delete the branch anyway, run:")
	assert.Contains(t, output.Stderr(), "git log "+cleanupHeadOID+".."+cleanupLocalTip+" && git branch -D feature")
	assert.NotContains(t, output.Stderr(), "Deleted local branch")
	// The remote branch is still part of the pull request, so it goes.
	assert.Contains(t, output.Stderr(), "Deleted remote branch feature")
}

func TestPrMerge_deleteBranch_localTipNotInPRInWorktree(t *testing.T) {
	// The safety guard has to withhold the worktree removal as well, not just
	// the branch delete, because the worktree is what holds the unpushed work.
	output, ran, err := runCleanupScenario(t, cleanupScenario{
		remotes:              remotesFor(cleanupPRBranch),
		headRefOid:           cleanupHeadOID,
		localTipAhead:        true,
		headBranchInWorktree: true,
	})
	require.NoError(t, err)

	assert.Empty(t, destructiveRan(ran))
	assert.NotContains(t, output.Stderr(), "Removed worktree")
	assert.Contains(t, output.Stderr(), "has commits that are not in pull request OWNER-A/REPO-A#7")
}

func TestPrMerge_deleteBranch_localTipInPR(t *testing.T) {
	// The regression guard: the pull request's own repository, with a local
	// "feature" branch contained in the pull request head, is still cleaned up.
	// Both guards have to let this through or --delete-branch stops working.
	output, ran, err := runCleanupScenario(t, cleanupScenario{
		remotes:       remotesFor(cleanupPRBranch),
		headRefOid:    cleanupHeadOID,
		localTipAhead: false,
	})
	require.NoError(t, err)

	assert.NotContains(t, output.Stderr(), "skipping local branch delete")
	assert.Contains(t, output.Stderr(), "Deleted local branch feature")
	assert.Contains(t, output.Stderr(), "Deleted remote branch feature")
	assert.True(t, ranSomething(ran, "branch -D feature"), "expected `git branch -D feature`, got %v", ran)
}

func TestPrMerge_deleteBranch_localTipInPRInWorktree(t *testing.T) {
	// The same, with the head branch checked out in a linked worktree, which is
	// the path the reporter's repository would have taken had the scope guard
	// not stopped it.
	output, ran, err := runCleanupScenario(t, cleanupScenario{
		remotes:              remotesFor(cleanupPRBranch),
		headRefOid:           cleanupHeadOID,
		headBranchInWorktree: true,
	})
	require.NoError(t, err)

	assert.Contains(t, output.Stderr(), "Removed worktree /path/to/feature-wt")
	assert.Contains(t, output.Stderr(), "Deleted local branch feature")
	assert.Contains(t, output.Stderr(), "Deleted remote branch feature")
	assert.True(t, ranSomething(ran, "worktree remove"), "expected the worktree to be removed, got %v", ran)
	assert.True(t, ranSomething(ran, "branch -D feature"), "expected `git branch -D feature`, got %v", ran)
}

func TestPrMerge_deleteBranch_ownerCaseMatchesRemote(t *testing.T) {
	// A clone of the pull request's own repository whose owner is spelled with
	// different casing, which GitHub treats as the same repository, is still in
	// scope. This is the counterpart to the different-case test above and keeps
	// the scope check from being stricter than it needs to be.
	output, ran, err := runCleanupScenario(t, cleanupScenario{
		remotes:    remotesFor("owner-a/repo-a"),
		headRefOid: cleanupHeadOID,
	})
	require.NoError(t, err)

	assert.NotContains(t, output.Stderr(), "skipping local branch delete")
	assert.Contains(t, output.Stderr(), "Deleted local branch feature")
	assert.True(t, ranSomething(ran, "branch -D feature"), "expected `git branch -D feature`, got %v", ran)
}

func TestPrMerge_deleteBranch_headRepositoryNotInScope(t *testing.T) {
	// Given a pull request from a fork, and a current directory that is a clone
	// of the fork rather than of the base repository.
	//
	// Then the head repository does NOT count as the pull request's repository,
	// and the local head branch is left alone. Only the base repository does, so
	// that the check cannot be satisfied by a name that merely collides with it.
	output, ran, err := runCleanupScenario(t, cleanupScenario{
		remotes:         remotesFor(cleanupForkRepo),
		headRefOid:      cleanupHeadOID,
		crossRepository: true,
	})
	require.NoError(t, err)

	assert.NotContains(t, output.String(), "Deleted local branch")
	assert.NotContains(t, output.String(), "Removed worktree")
	assert.NotContains(t, output.Stderr(), "Deleted local branch")
	assert.NotContains(t, output.Stderr(), "Removed worktree")

	// A cross-repository pull request leaves the remote branch alone.
	assert.NotContains(t, output.Stderr(), "Deleted remote branch")
	assert.False(t, ranSomething(ran, "branch -D feature"), "expected no `git branch -D`, got %v", ran)
}

func TestPrMerge_deleteBranch_baseRepositoryInScopeForFork(t *testing.T) {
	// The common fork workflow: the user sits in the base repository, which is
	// the pull request's own repository even though the head branch is not.
	output, _, err := runCleanupScenario(t, cleanupScenario{
		remotes:         remotesFor(cleanupPRBranch),
		headRefOid:      cleanupHeadOID,
		crossRepository: true,
	})
	require.NoError(t, err)

	assert.NotContains(t, output.Stderr(), "skipping local branch delete")
	assert.Contains(t, output.Stderr(), "Deleted local branch feature")
}
