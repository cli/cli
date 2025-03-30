package sync

import (
	"context"
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
