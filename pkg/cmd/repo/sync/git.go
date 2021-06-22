package sync

import (
	"github.com/cli/cli/git"
)

type gitClient interface {
	Checkout([]string) error
	CurrentBranch() (string, error)
	Fetch([]string) error
	HasLocalBranch([]string) bool
	IsAncestor([]string) (bool, error)
	IsDirty() (bool, error)
	Merge([]string) error
	Reset([]string) error
}

type gitExecuter struct{}

func (g *gitExecuter) Checkout(args []string) error {
	return git.CheckoutBranch(args[0])
}

func (g *gitExecuter) CurrentBranch() (string, error) {
	return git.CurrentBranch()
}

func (g *gitExecuter) Fetch(args []string) error {
	args = append([]string{"fetch"}, args...)
	cmd, err := git.GitCommand(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (g *gitExecuter) HasLocalBranch(args []string) bool {
	return git.HasLocalBranch(args[0])
}

func (g *gitExecuter) IsAncestor(args []string) (bool, error) {
	args = append([]string{"merge-base", "--is-ancestor"}, args...)
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

func (g *gitExecuter) Merge(args []string) error {
	args = append([]string{"merge"}, args...)
	cmd, err := git.GitCommand(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (g *gitExecuter) Reset(args []string) error {
	args = append([]string{"reset"}, args...)
	cmd, err := git.GitCommand(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (g *gitExecuter) Stash(args []string) error {
	args = append([]string{"stash"}, args...)
	cmd, err := git.GitCommand(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}
