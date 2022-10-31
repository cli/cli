package extension

import (
	"context"

	"github.com/cli/cli/v2/git"
)

type gitClient interface {
	CheckoutBranch(branch string, mods ...git.CommandModifier) error
	Clone(cloneURL string, args []string, mods ...git.CommandModifier) (string, error)
	CommandOutput(args []string, mods ...git.CommandModifier) ([]byte, error)
	Config(name string, mods ...git.CommandModifier) (string, error)
	Fetch(remote string, refspec string, mods ...git.CommandModifier) error
	Pull(remote, branch string, mods ...git.CommandModifier) error
	Remotes(mods ...git.CommandModifier) (git.RemoteSet, error)
}

type gitExecuter struct {
	client *git.Client
}

func (g *gitExecuter) CheckoutBranch(branch string, mods ...git.CommandModifier) error {
	return g.client.CheckoutBranch(context.Background(), branch, mods...)
}

func (g *gitExecuter) Clone(cloneURL string, cloneArgs []string, mods ...git.CommandModifier) (string, error) {
	return g.client.Clone(context.Background(), cloneURL, cloneArgs, mods...)
}

func (g *gitExecuter) CommandOutput(args []string, mods ...git.CommandModifier) ([]byte, error) {
	cmd, err := g.client.Command(context.Background(), args...)
	if err != nil {
		return nil, err
	}
	for _, mod := range mods {
		mod(cmd)
	}
	return cmd.Output()
}

func (g *gitExecuter) Config(name string, mods ...git.CommandModifier) (string, error) {
	return g.client.Config(context.Background(), name, mods...)
}

func (g *gitExecuter) Fetch(remote string, refspec string, mods ...git.CommandModifier) error {
	return g.client.Fetch(context.Background(), remote, refspec, mods...)
}

func (g *gitExecuter) Pull(remote, branch string, mods ...git.CommandModifier) error {
	return g.client.Pull(context.Background(), remote, branch, mods...)
}

func (g *gitExecuter) Remotes(mods ...git.CommandModifier) (git.RemoteSet, error) {
	return g.client.Remotes(context.Background(), mods...)
}
