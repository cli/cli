package git

import (
	"context"
)

func GitCommand(args ...string) (*gitCommand, error) {
	c := &Client{}
	return c.Command(context.Background(), args...)
}

func RemotesForPath(repoDir string) (RemoteSet, error) {
	c := &Client{
		RepoDir: repoDir,
	}
	return c.Remotes(context.Background())
}
