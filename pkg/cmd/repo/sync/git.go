package sync

import (
	"context"
	"strings"
	"fmt"

	"github.com/cli/cli/v2/git"
)

type gitClient interface {
	CurrentBranch() (string, error)
	UpdateBranch(string, string) error
	CreateBranch(string, string, string) error
	Fetch(string, string) error
	HasLocalBranch(string) bool
	IsAncestor(string, string) (bool, error)
	IsCheckedOutInOtherWorktree(string) (bool, error)
	IsDirty() (bool, error)
	MergeFastForward(string) error
	ResetHard(string) error
}

type gitExecuter struct {
	client *git.Client
}

func (g *gitExecuter) UpdateBranch(branch, ref string) error {
	cmd, err := g.client.Command(context.Background(), "update-ref", fmt.Sprintf("refs/heads/%s", branch), ref)
	if err != nil {
		return err
	}
	_, err = cmd.Output()
	return err
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

// IsCheckedOutInOtherWorktree returns true if the given branch is checked
// out in any git worktree other than the current one. This is used to avoid
// advancing a branch ref (via `git update-ref`) while another worktree has
// that branch checked out, which would silently leave that worktree's index
// and working tree out of sync with the new ref. See cli/cli#12927.
func (g *gitExecuter) IsCheckedOutInOtherWorktree(branch string) (bool, error) {
	cmd, err := g.client.Command(context.Background(), "worktree", "list", "--porcelain")
	if err != nil {
		return false, err
	}
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}

	// `git worktree list --porcelain` emits one record per worktree separated
	// by a blank line. Each record starts with `worktree <path>` and, for
	// worktrees with a checked-out branch, contains `branch refs/heads/<name>`.
	// We scan for the target branch in a worktree other than the current one.
	currentWorktree, err := g.currentWorktreePath()
	if err != nil {
		return false, err
	}

	target := "branch refs/heads/" + branch
	var wt string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			wt = strings.TrimPrefix(line, "worktree ")
		} else if line == target && wt != currentWorktree {
			return true, nil
		}
	}
	return false, nil
}

// currentWorktreePath returns the absolute path of the current git worktree.
func (g *gitExecuter) currentWorktreePath() (string, error) {
	cmd, err := g.client.Command(context.Background(), "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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
