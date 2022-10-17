package sync

import (
	"context"
	"fmt"
	"strings"

	"github.com/cli/cli/v2/git"
)

type gitClient interface {
	BranchRemote(string) (string, error)
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

func (g *gitExecuter) BranchRemote(branch string) (string, error) {
	args := []string{"rev-parse", "--symbolic-full-name", "--abbrev-ref", fmt.Sprintf("%s@{u}", branch)}
	cmd, err := g.client.Command(context.Background(), args...)
	if err != nil {
		return "", err
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	parts := strings.SplitN(string(out), "/", 2)
	return parts[0], nil
}

func (g *gitExecuter) UpdateBranch(branch, ref string) error {
	cmd, err := g.client.Command(context.Background(), "update-ref", fmt.Sprintf("refs/heads/%s", branch), ref)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (g *gitExecuter) CreateBranch(branch, ref, upstream string) error {
	cmd, err := g.client.Command(context.Background(), "branch", branch, ref)
	if err != nil {
		return err
	}
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd, err = g.client.Command(context.Background(), "branch", "--set-upstream-to", upstream, branch)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (g *gitExecuter) CurrentBranch() (string, error) {
	return g.client.CurrentBranch(context.Background())
}

func (g *gitExecuter) Fetch(remote, ref string) error {
	args := []string{"fetch", "-q", remote, ref}
	cmd, err := g.client.Command(context.Background(), args...)
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
	err = cmd.Run()
	return err == nil, nil
}

func (g *gitExecuter) IsDirty() (bool, error) {
	cmd, err := g.client.Command(context.Background(), "status", "--untracked-files=no", "--porcelain")
	if err != nil {
		return false, err
	}
	output, err := cmd.Output()
	if err != nil {
		return true, err
	}
	if len(output) > 0 {
		return true, nil
	}
	return false, nil
}

func (g *gitExecuter) MergeFastForward(ref string) error {
	args := []string{"merge", "--ff-only", "--quiet", ref}
	cmd, err := g.client.Command(context.Background(), args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (g *gitExecuter) ResetHard(ref string) error {
	args := []string{"reset", "--hard", ref}
	cmd, err := g.client.Command(context.Background(), args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}
