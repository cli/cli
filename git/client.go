package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/safeexec"
)

var remoteRE = regexp.MustCompile(`(.+)\s+(.+)\s+\((push|fetch)\)`)

// ErrNotOnAnyBranch indicates that the user is in detached HEAD state.
var ErrNotOnAnyBranch = errors.New("git: not on any branch")

type NotInstalled struct {
	message string
	err     error
}

func (e *NotInstalled) Error() string {
	return e.message
}

func (e *NotInstalled) Unwrap() error {
	return e.err
}

type GitError struct {
	stderr string
	err    error
}

func (ge *GitError) Error() string {
	stderr := ge.stderr
	if stderr == "" {
		var exitError *exec.ExitError
		if errors.As(ge.err, &exitError) {
			stderr = string(exitError.Stderr)
		}
	}
	if stderr == "" {
		return fmt.Sprintf("failed to run git: %v", ge.err)
	}
	return fmt.Sprintf("failed to run git: %s", stderr)
}

func (ge *GitError) Unwrap() error {
	return ge.err
}

type gitCommand struct {
	*exec.Cmd
}

// This is a hack in order to not break the hundreds of
// existing tests that rely on `run.PrepareCmd` to be invoked.
func (gc *gitCommand) Run() error {
	return run.PrepareCmd(gc.Cmd).Run()
}

// This is a hack in order to not break the hundreds of
// existing tests that rely on `run.PrepareCmd` to be invoked.
func (gc *gitCommand) Output() ([]byte, error) {
	return run.PrepareCmd(gc.Cmd).Output()
}

type Client struct {
	GhPath  string
	RepoDir string
	GitPath string
	Stderr  io.Writer
	Stdin   io.Reader
	Stdout  io.Writer

	commandContext func(ctx context.Context, name string, args ...string) *exec.Cmd
	mu             sync.Mutex
}

func (c *Client) Command(ctx context.Context, args ...string) (*gitCommand, error) {
	if c.RepoDir != "" {
		args = append([]string{"-C", c.RepoDir}, args...)
	}
	commandContext := exec.CommandContext
	if c.commandContext != nil {
		commandContext = c.commandContext
	}
	var err error
	c.mu.Lock()
	if c.GitPath == "" {
		c.GitPath, err = resolveGitPath()
	}
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	cmd := commandContext(ctx, c.GitPath, args...)
	cmd.Stderr = c.Stderr
	cmd.Stdin = c.Stdin
	cmd.Stdout = c.Stdout
	return &gitCommand{cmd}, nil
}

func resolveGitPath() (string, error) {
	path, err := safeexec.LookPath("git")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			programName := "git"
			if runtime.GOOS == "windows" {
				programName = "Git for Windows"
			}
			return "", &NotInstalled{
				message: fmt.Sprintf("unable to find git executable in PATH; please install %s before retrying", programName),
				err:     err,
			}
		}
		return "", err
	}
	return path, nil
}

// AuthenticatedCommand is a wrapper around Command that included configuration to use gh
// as the credential helper for git.
func (c *Client) AuthenticatedCommand(ctx context.Context, args ...string) (*gitCommand, error) {
	preArgs := []string{}
	preArgs = append(preArgs, "-c", "credential.helper=")
	if c.GhPath == "" {
		// Assumes that gh is in PATH.
		c.GhPath = "gh"
	}
	credHelper := fmt.Sprintf("!%q auth git-credential", c.GhPath)
	preArgs = append(preArgs, "-c", fmt.Sprintf("credential.helper=%s", credHelper))
	args = append(preArgs, args...)
	return c.Command(ctx, args...)
}

func (c *Client) Remotes(ctx context.Context) (RemoteSet, error) {
	remoteArgs := []string{"remote", "-v"}
	remoteCmd, err := c.Command(ctx, remoteArgs...)
	if err != nil {
		return nil, err
	}
	remoteOut, remoteErr := remoteCmd.Output()
	if remoteErr != nil {
		return nil, &GitError{err: remoteErr}
	}

	configArgs := []string{"config", "--get-regexp", `^remote\..*\.gh-resolved$`}
	configCmd, err := c.Command(ctx, configArgs...)
	if err != nil {
		return nil, err
	}
	configOut, configErr := configCmd.Output()
	if configErr != nil {
		// Ignore exit code 1 as it means there are no resolved remotes.
		var exitErr *exec.ExitError
		if errors.As(configErr, &exitErr) && exitErr.ExitCode() != 1 {
			return nil, &GitError{err: configErr}
		}
	}

	remotes := parseRemotes(outputLines(remoteOut))
	populateResolvedRemotes(remotes, outputLines(configOut))
	sort.Sort(remotes)
	return remotes, nil
}

func (c *Client) AddRemote(ctx context.Context, name, urlStr string, trackingBranches []string) (*Remote, error) {
	args := []string{"remote", "add"}
	for _, branch := range trackingBranches {
		args = append(args, "-t", branch)
	}
	args = append(args, "-f", name, urlStr)
	//TODO: Use AuthenticatedCommand
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return nil, err
	}
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	var urlParsed *url.URL
	if strings.HasPrefix(urlStr, "https") {
		urlParsed, err = url.Parse(urlStr)
		if err != nil {
			return nil, err
		}
	} else {
		urlParsed, err = ParseURL(urlStr)
		if err != nil {
			return nil, err
		}
	}
	remote := &Remote{
		Name:     name,
		FetchURL: urlParsed,
		PushURL:  urlParsed,
	}
	return remote, nil
}

func (c *Client) UpdateRemoteURL(ctx context.Context, name, url string) error {
	args := []string{"remote", "set-url", name, url}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (c *Client) SetRemoteResolution(ctx context.Context, name, resolution string) error {
	args := []string{"config", "--add", fmt.Sprintf("remote.%s.gh-resolved", name), resolution}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

// CurrentBranch reads the checked-out branch for the git repository.
func (c *Client) CurrentBranch(ctx context.Context) (string, error) {
	args := []string{"symbolic-ref", "--quiet", "HEAD"}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return "", err
	}
	errBuf := bytes.Buffer{}
	cmd.Stderr = &errBuf
	out, err := cmd.Output()
	if err != nil {
		if errBuf.Len() == 0 {
			return "", &GitError{err: err, stderr: "not on any branch"}
		}
		return "", &GitError{err: err, stderr: errBuf.String()}
	}
	branch := firstLine(out)
	return strings.TrimPrefix(branch, "refs/heads/"), nil
}

// ShowRefs resolves fully-qualified refs to commit hashes.
func (c *Client) ShowRefs(ctx context.Context, ref ...string) ([]Ref, error) {
	args := append([]string{"show-ref", "--verify", "--"}, ref...)
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return nil, err
	}
	// This functionality relies on parsing output from the git command despite
	// an error status being returned from git.
	out, err := cmd.Output()
	var refs []Ref
	for _, line := range outputLines(out) {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		refs = append(refs, Ref{
			Hash: parts[0],
			Name: parts[1],
		})
	}
	return refs, err
}

func (c *Client) Config(ctx context.Context, name string) (string, error) {
	args := []string{"config", name}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return "", err
	}
	errBuf := bytes.Buffer{}
	cmd.Stderr = &errBuf
	out, err := cmd.Output()
	if err != nil {
		var exitError *exec.ExitError
		if ok := errors.As(err, &exitError); ok && exitError.Error() == "1" {
			return "", &GitError{err: err, stderr: fmt.Sprintf("unknown config key %s", name)}
		}
		return "", &GitError{err: err, stderr: errBuf.String()}
	}
	return firstLine(out), nil
}

func (c *Client) UncommittedChangeCount(ctx context.Context) (int, error) {
	args := []string{"status", "--porcelain"}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return 0, err
	}
	out, err := cmd.Output()
	if err != nil {
		return 0, &GitError{err: err}
	}
	lines := strings.Split(string(out), "\n")
	count := 0
	for _, l := range lines {
		if l != "" {
			count++
		}
	}
	return count, nil
}

func (c *Client) Commits(ctx context.Context, baseRef, headRef string) ([]*Commit, error) {
	args := []string{"-c", "log.ShowSignature=false", "log", "--pretty=format:%H,%s", "--cherry", fmt.Sprintf("%s...%s", baseRef, headRef)}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return nil, err
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, &GitError{err: err}
	}
	commits := []*Commit{}
	sha := 0
	title := 1
	for _, line := range outputLines(out) {
		split := strings.SplitN(line, ",", 2)
		if len(split) != 2 {
			continue
		}
		commits = append(commits, &Commit{
			Sha:   split[sha],
			Title: split[title],
		})
	}
	if len(commits) == 0 {
		return nil, fmt.Errorf("could not find any commits between %s and %s", baseRef, headRef)
	}
	return commits, nil
}

func (c *Client) lookupCommit(ctx context.Context, sha, format string) ([]byte, error) {
	args := []string{"-c", "log.ShowSignature=false", "show", "-s", "--pretty=format:" + format, sha}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return nil, err
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, &GitError{err: err}
	}
	return out, nil
}

func (c *Client) LastCommit(ctx context.Context) (*Commit, error) {
	output, err := c.lookupCommit(ctx, "HEAD", "%H,%s")
	if err != nil {
		return nil, err
	}
	idx := bytes.IndexByte(output, ',')
	return &Commit{
		Sha:   string(output[0:idx]),
		Title: strings.TrimSpace(string(output[idx+1:])),
	}, nil
}

func (c *Client) CommitBody(ctx context.Context, sha string) (string, error) {
	output, err := c.lookupCommit(ctx, sha, "%b")
	return string(output), err
}

// Push publishes a git ref to a remote and sets up upstream configuration.
func (c *Client) Push(ctx context.Context, remote string, ref string) error {
	args := []string{"push", "--set-upstream", remote, ref}
	//TODO: Use AuthenticatedCommand
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

// ReadBranchConfig parses the `branch.BRANCH.(remote|merge)` part of git config.
func (c *Client) ReadBranchConfig(ctx context.Context, branch string) (cfg BranchConfig) {
	prefix := regexp.QuoteMeta(fmt.Sprintf("branch.%s.", branch))
	args := []string{"config", "--get-regexp", fmt.Sprintf("^%s(remote|merge)$", prefix)}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return
	}
	out, err := cmd.Output()
	if err != nil {
		return
	}
	for _, line := range outputLines(out) {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		keys := strings.Split(parts[0], ".")
		switch keys[len(keys)-1] {
		case "remote":
			if strings.Contains(parts[1], ":") {
				u, err := ParseURL(parts[1])
				if err != nil {
					continue
				}
				cfg.RemoteURL = u
			} else if !isFilesystemPath(parts[1]) {
				cfg.RemoteName = parts[1]
			}
		case "merge":
			cfg.MergeRef = parts[1]
		}
	}
	return
}

func (c *Client) DeleteLocalBranch(ctx context.Context, branch string) error {
	args := []string{"branch", "-D", branch}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (c *Client) HasLocalBranch(ctx context.Context, branch string) bool {
	args := []string{"rev-parse", "--verify", "refs/heads/" + branch}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return false
	}
	err = cmd.Run()
	return err == nil
}

func (c *Client) CheckoutBranch(ctx context.Context, branch string) error {
	args := []string{"checkout", branch}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (c *Client) CheckoutNewBranch(ctx context.Context, remoteName, branch string) error {
	track := fmt.Sprintf("%s/%s", remoteName, branch)
	args := []string{"checkout", "-b", branch, "--track", track}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (c *Client) Pull(ctx context.Context, remote, branch string) error {
	args := []string{"pull", "--ff-only", remote, branch}
	//TODO: Use AuthenticatedCommand
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (c *Client) Clone(ctx context.Context, cloneURL string, args []string) (target string, err error) {
	cloneArgs, target := parseCloneArgs(args)
	cloneArgs = append(cloneArgs, cloneURL)
	// If the args contain an explicit target, pass it to clone
	// otherwise, parse the URL to determine where git cloned it to so we can return it
	if target != "" {
		cloneArgs = append(cloneArgs, target)
	} else {
		target = path.Base(strings.TrimSuffix(cloneURL, ".git"))
	}
	cloneArgs = append([]string{"clone"}, cloneArgs...)
	//TODO: Use AuthenticatedCommand
	cmd, err := c.Command(ctx, cloneArgs...)
	if err != nil {
		return "", err
	}
	err = cmd.Run()
	return
}

// ToplevelDir returns the top-level directory path of the current repository.
func (c *Client) ToplevelDir(ctx context.Context) (string, error) {
	args := []string{"rev-parse", "--show-toplevel"}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return "", err
	}
	out, err := cmd.Output()
	if err != nil {
		return "", &GitError{err: err}
	}
	return firstLine(out), nil
}

func (c *Client) GitDir(ctx context.Context) (string, error) {
	args := []string{"rev-parse", "--git-dir"}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return "", err
	}
	out, err := cmd.Output()
	if err != nil {
		return "", &GitError{err: err}
	}
	return firstLine(out), nil
}

// Show current directory relative to the top-level directory of repository.
func (c *Client) PathFromRoot(ctx context.Context) string {
	args := []string{"rev-parse", "--show-prefix"}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return ""
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	if path := firstLine(out); path != "" {
		return path[:len(path)-1]
	}
	return ""
}

func isFilesystemPath(p string) bool {
	return p == "." || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "/")
}

func outputLines(output []byte) []string {
	lines := strings.TrimSuffix(string(output), "\n")
	return strings.Split(lines, "\n")
}

func firstLine(output []byte) string {
	if i := bytes.IndexAny(output, "\n"); i >= 0 {
		return string(output)[0:i]
	}
	return string(output)
}

func parseCloneArgs(extraArgs []string) (args []string, target string) {
	args = extraArgs
	if len(args) > 0 {
		if !strings.HasPrefix(args[0], "-") {
			target, args = args[0], args[1:]
		}
	}
	return
}

func parseRemotes(remotesStr []string) RemoteSet {
	remotes := RemoteSet{}
	for _, r := range remotesStr {
		match := remoteRE.FindStringSubmatch(r)
		if match == nil {
			continue
		}
		name := strings.TrimSpace(match[1])
		urlStr := strings.TrimSpace(match[2])
		urlType := strings.TrimSpace(match[3])

		url, err := ParseURL(urlStr)
		if err != nil {
			continue
		}

		var rem *Remote
		if len(remotes) > 0 {
			rem = remotes[len(remotes)-1]
			if name != rem.Name {
				rem = nil
			}
		}
		if rem == nil {
			rem = &Remote{Name: name}
			remotes = append(remotes, rem)
		}

		switch urlType {
		case "fetch":
			rem.FetchURL = url
		case "push":
			rem.PushURL = url
		}
	}
	return remotes
}

func populateResolvedRemotes(remotes RemoteSet, resolved []string) {
	for _, l := range resolved {
		parts := strings.SplitN(l, " ", 2)
		if len(parts) < 2 {
			continue
		}
		rp := strings.SplitN(parts[0], ".", 3)
		if len(rp) < 2 {
			continue
		}
		name := rp[1]
		for _, r := range remotes {
			if r.Name == name {
				r.Resolved = parts[1]
				break
			}
		}
	}
}
