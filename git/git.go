package git

import (
	"context"
	"os"
)

func GitCommand(args ...string) (*gitCommand, error) {
	c := &Client{}
	return c.Command(context.Background(), args...)
}

func RunClone(cloneURL string, args []string) (target string, err error) {
	//TODO: Replace with factory GitClient and use AuthenticatedCommand
	c := &Client{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	return c.Clone(context.Background(), cloneURL, args)
}

func GetDirFromPath(repoDir string) (string, error) {
	c := &Client{
		RepoDir: repoDir,
	}
	return c.GitDir(context.Background())
}

func RemotesForPath(repoDir string) (RemoteSet, error) {
	c := &Client{
		RepoDir: repoDir,
	}
	return c.Remotes(context.Background())
}

func AddNamedRemote(url, name, repoDir string, branches []string) error {
	c := &Client{
		RepoDir: repoDir,
	}
	_, err := c.AddRemote(context.Background(), name, url, branches)
	return err
}
