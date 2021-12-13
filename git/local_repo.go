package git

import (
	"bytes"
	"context"
	"os/exec"
	"strings"

	"github.com/cli/safeexec"
)

type LocalRepo struct {
	gitExePath string
	gitExeErr  error
}

func (c *LocalRepo) PathPrefix(ctx context.Context) (string, error) {
	showCmd, err := c.gitCommand(ctx, "rev-parse", "--show-prefix")
	if err != nil {
		return "", err
	}
	output, err := showCmd.Output()
	return strings.TrimRight(string(output), "\n"), err
}

func (c *LocalRepo) LastCommit(ctx context.Context) (*Commit, error) {
	showCmd, err := c.gitCommand(ctx, "-c", "log.ShowSignature=false", "show", "-s", "--pretty=format:%H,%s", "HEAD")
	if err != nil {
		return nil, err
	}
	output, err := showCmd.Output()
	if err != nil {
		return nil, err
	}
	idx := bytes.IndexByte(output, ',')
	return &Commit{
		Sha:   string(output[0:idx]),
		Title: strings.TrimSpace(string(output[idx+1:])),
	}, nil
}

func (c *LocalRepo) gitCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	gitExe, err := c.gitExe()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, gitExe, args...)
	return cmd, nil
}

func (c *LocalRepo) gitExe() (string, error) {
	if c.gitExePath == "" && c.gitExeErr == nil {
		c.gitExePath, c.gitExeErr = safeexec.LookPath("git")
	}
	return c.gitExePath, c.gitExeErr
}
