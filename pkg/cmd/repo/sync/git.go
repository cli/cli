package sync

import (
	"context"
	"strings"

	"github.com/cli/cli/v2/git"
)

type gitClient interface {
	BranchWorktreePath(string) (string, error)
	CurrentBranch() (string, error)
	UpdateBranch(string, string) error
	CreateBranch(string, string, string) error
	Fetch(string, string) error
	HasLocalBranch(string) bool
	IsAncestor(string, string) (bool, error)
	IsDirty() (bool, error)
	MergeFastForward(string) error
	ResetHard(string) error
}

type gitExecuter struct {
	client *git.Client
}

// UpdateBranch moves branch to ref. It uses `git branch --force` rather than
// `git update-ref` because update-ref will happily move a branch that is checked out in
// another worktree, leaving that worktree's index and working tree stale. Callers should
// use BranchWorktreePath to detect and explain that case first; this is the safety net
// for states that check can't see, such as an in-progress bisect.
func (g *gitExecuter) UpdateBranch(branch, ref string) error {
	cmd, err := g.client.Command(context.Background(), "branch", "--force", "--", branch, ref)
	if err != nil {
		return err
	}
	_, err = cmd.Output()
	return err
}

// BranchWorktreePath returns the path of the worktree in which branch is checked out,
// or an empty string when no worktree has it checked out.
func (g *gitExecuter) BranchWorktreePath(branch string) (string, error) {
	cmd, err := g.client.Command(context.Background(), "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return worktreePathForBranch(string(out), branch), nil
}

// worktreePathForBranch parses `git worktree list --porcelain` output, which lists each
// worktree as a `worktree <path>` line followed by attributes such as `branch <ref>`.
func worktreePathForBranch(porcelain, branch string) string {
	wantRef := "refs/heads/" + branch

	var path string
	for _, line := range strings.Split(porcelain, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if rest, ok := strings.CutPrefix(line, "worktree "); ok {
			path = rest
		} else if rest, ok := strings.CutPrefix(line, "branch "); ok && rest == wantRef {
			return path
		}
	}
	return ""
}

func (g *gitExecuter) CreateBranch(branch, ref, upstream string) error {
	ctx := context.Background()
	cmd, err := g.client.Command(ctx, "branch", branch, ref)
	if err != nil {
		return err
	}
	if _, err := cmd.Output(); err != nil {
		return err
	}
	cmd, err = g.client.Command(ctx, "branch", "--set-upstream-to", upstream, branch)
	if err != nil {
		return err
	}
	_, err = cmd.Output()
	return err
}

func (g *gitExecuter) CurrentBranch() (string, error) {
	return g.client.CurrentBranch(context.Background())
}

func (g *gitExecuter) Fetch(remote, ref string) error {
	args := []string{"fetch", "-q", remote, ref}
	cmd, err := g.client.AuthenticatedCommand(context.Background(), git.AllMatchingCredentialsPattern, args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (g *gitExecuter) HasLocalBranch(branch string) bool {
	return g.client.HasLocalBranch(context.Background(), branch)
}

func (g *gitExecuter) IsAncestor(ancestor, progeny string) (bool, error) {
	args := []string{"merge-base", "--is-ancestor", ancestor, progeny}
	cmd, err := g.client.Command(context.Background(), args...)
	if err != nil {
		return false, err
	}
	_, err = cmd.Output()
	return err == nil, nil
}

func (g *gitExecuter) IsDirty() (bool, error) {
	changeCount, err := g.client.UncommittedChangeCount(context.Background())
	if err != nil {
		return false, err
	}
	return changeCount != 0, nil
}

func (g *gitExecuter) MergeFastForward(ref string) error {
	args := []string{"merge", "--ff-only", "--quiet", ref}
	cmd, err := g.client.Command(context.Background(), args...)
	if err != nil {
		return err
	}
	_, err = cmd.Output()
	return err
}

func (g *gitExecuter) ResetHard(ref string) error {
	args := []string{"reset", "--hard", ref}
	cmd, err := g.client.Command(context.Background(), args...)
	if err != nil {
		return err
	}
	_, err = cmd.Output()
	return err
}
