package sync

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/pkg/iostreams"
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
	io *iostreams.IOStreams
}

func (g *gitExecuter) BranchRemote(branch string) (string, error) {
	args := []string{"rev-parse", "--symbolic-full-name", "--abbrev-ref", fmt.Sprintf("%s@{u}", branch)}
	cmd, err := git.GitCommand(args...)
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
	cmd, err := git.GitCommand("update-ref", fmt.Sprintf("refs/heads/%s", branch), ref)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (g *gitExecuter) CreateBranch(branch, ref, upstream string) error {
	cmd, err := git.GitCommand("branch", branch, ref)
	if err != nil {
		return err
	}
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd, err = git.GitCommand("branch", "--set-upstream-to", upstream, branch)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (g *gitExecuter) CurrentBranch() (string, error) {
	return git.CurrentBranch()
}

func (g *gitExecuter) Fetch(remote, ref string) error {
	args := []string{"fetch", "-q", remote, ref}
	cmd, err := git.GitCommand(args...)
	if err != nil {
		return err
	}
	cmd.Stdin = g.io.In
	cmd.Stdout = g.io.Out
	cmd.Stderr = g.io.ErrOut
	return cmd.Run()
}

func (g *gitExecuter) HasLocalBranch(branch string) bool {
	return git.HasLocalBranch(branch)
}

func (g *gitExecuter) IsAncestor(ancestor, progeny string) (bool, error) {
	args := []string{"merge-base", "--is-ancestor", ancestor, progeny}
	cmd, err := git.GitCommand(args...)
	if err != nil {
		return false, err
	}
	err = cmd.Run()
	return err == nil, nil
}

func (g *gitExecuter) IsDirty() (bool, error) {
	cmd, err := git.GitCommand("status", "--untracked-files=no", "--porcelain")
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
	cmd, err := git.GitCommand(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (g *gitExecuter) ResetHard(ref string) error {
	args := []string{"reset", "--hard", ref}
	cmd, err := git.GitCommand(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}
