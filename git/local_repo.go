package git

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os/exec"
	"regexp"
	"strings"

	"github.com/cli/safeexec"
)

type LocalRepo struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	gitCommandInit func(ctx context.Context, args ...string) (*exec.Cmd, error)
	gitExePath     string
	gitExeErr      error
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
	if c.gitCommandInit != nil {
		return c.gitCommandInit(ctx, args...)
	}
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

type PushOptions struct {
	RemoteName   string
	TargetBranch string
	ShowProgress bool
	SetUpstream  bool
}

var gitPushRegexp = regexp.MustCompile(`^remote: (Create a pull request.*by visiting|[[:space:]]*https://.*/pull/new/)`)

// Push publishes a branch to a git remote and filters out "Create a pull request by visiting <URL>" lines
// before forwarding them to stderr.
func (g *LocalRepo) Push(ctx context.Context, opts PushOptions) error {
	args := []string{"push"}
	if opts.SetUpstream {
		args = append(args, "--set-upstream")
	}
	if opts.ShowProgress {
		args = append(args, "--progress")
	}
	args = append(args, opts.RemoteName, "HEAD:"+opts.TargetBranch)

	pushCmd, err := g.gitCommand(ctx, args...)
	if err != nil {
		return err
	}
	pushCmd.Stdin = g.Stdin
	pushCmd.Stdout = g.Stdout
	r, err := pushCmd.StderrPipe()
	if err != nil {
		return err
	}
	if err = pushCmd.Start(); err != nil {
		return err
	}
	if err = filterLines(g.Stderr, r, gitPushRegexp); err != nil {
		return err
	}
	return pushCmd.Wait()
}

func filterLines(w io.Writer, r io.Reader, re *regexp.Regexp) error {
	s := bufio.NewScanner(r)
	s.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		i := bytes.IndexAny(data, "\r\n")
		if i >= 0 {
			// encompass both CR & LF characters if they appear together
			if data[i] == '\r' && len(data) > i+1 && data[i+1] == '\n' {
				return i + 2, data[0 : i+2], nil
			}
			return i + 1, data[0 : i+1], nil
		}
		if atEOF {
			return len(data), data, nil
		}
		// Request more data.
		return 0, nil, nil
	})
	for s.Scan() {
		line := s.Bytes()
		if !re.Match(line) {
			_, err := w.Write(line)
			if err != nil {
				return err
			}
		}
	}
	return s.Err()
}
