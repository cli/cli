package sync

import (
	"context"
	"fmt"
	"strings"

	"github.com/cli/cli/v2/git"
)

type gitClient interface {
	CurrentBranch() (string, error)
	UpdateBranch(string, string) error
	CreateBranch(string, string, string) error
	Fetch(string, string) error
	HasLocalBranch(string) bool
	IsAncestor(string, string) (bool, error)
	IsDirty() (bool, error)
	IsBranchCheckedOutInAnyWorktree(string) (bool, error)
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

// IsBranchCheckedOutInAnyWorktree checks if the given branch is checked out
// in any git worktree (not just the current one). This prevents silent corruption
// when using git update-ref on a branch that's checked out elsewhere.
func (g *gitExecuter) IsBranchCheckedOutInAnyWorktree(branch string) (bool, error) {
	args := []string{"worktree", "list", "--porcelain"}
	cmd, err := g.client.Command(context.Background(), args...)
	if err != nil {
		return false, err
	}
	output, err := cmd.Output()
	if err != nil {
		// If worktree command fails (e.g., older git), fall back to false
		// which means we can't detect it, but won't block the operation
		return false, nil
	}

	// Parse porcelain output: each worktree entry has "branch refs/heads/xxx" lines
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimPrefix(line, "branch ")
			// Match refs/heads/<branch>
			if ref == fmt.Sprintf("refs/heads/%s", branch) {
				return true, nil
			}
		}
	}
	return false, nil
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
