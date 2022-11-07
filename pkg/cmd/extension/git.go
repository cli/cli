package extension

import (
	"context"

	"github.com/cli/cli/v2/git"
)

type gitClient interface {
	CheckoutBranch(branch string) error
	Clone(cloneURL string, args []string) (string, error)
	CommandOutput(args []string) ([]byte, error)
	Config(name string) (string, error)
	Fetch(remote string, refspec string) error
	ForRepo(repoDir string) gitClient
	Pull(remote, branch string) error
	Remotes() (git.RemoteSet, error)
}

type gitExecuter struct {
	client *git.Client
}

func (g *gitExecuter) CheckoutBranch(branch string) error {
	return g.client.CheckoutBranch(context.Background(), branch)
}

func (g *gitExecuter) Clone(cloneURL string, cloneArgs []string) (string, error) {
	return g.client.Clone(context.Background(), cloneURL, cloneArgs)
}

func (g *gitExecuter) CommandOutput(args []string) ([]byte, error) {
	cmd, err := g.client.Command(context.Background(), args...)
	if err != nil {
		return nil, err
	}
	return cmd.Output()
}

func (g *gitExecuter) Config(name string) (string, error) {
	return g.client.Config(context.Background(), name)
}

func (g *gitExecuter) Fetch(remote string, refspec string) error {
	return g.client.Fetch(context.Background(), remote, refspec)
}

func (g *gitExecuter) ForRepo(repoDir string) gitClient {
	return &gitExecuter{
		client: &git.Client{
			GhPath:  g.client.GhPath,
			RepoDir: repoDir,
			GitPath: g.client.GitPath,
			Stderr:  g.client.Stderr,
			Stdin:   g.client.Stdin,
			Stdout:  g.client.Stdout,
		},
	}
}

func (g *gitExecuter) Pull(remote, branch string) error {
	return g.client.Pull(context.Background(), remote, branch)
}

func (g *gitExecuter) Remotes() (git.RemoteSet, error) {
	return g.client.Remotes(context.Background())
}
