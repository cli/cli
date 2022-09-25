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

	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/safeexec"
)

var remoteRE = regexp.MustCompile(`(.+)\s+(.+)\s+\((push|fetch)\)`)

// ErrNotOnAnyBranch indicates that the user is in detached HEAD state.
var ErrNotOnAnyBranch = errors.New("git: not on any branch")

type NotInstalled struct {
	message string
	error
}

func (e *NotInstalled) Error() string {
	return e.message
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
	GhPath    string
	RepoDir   string
	GitPath   string
	AuthHosts []string
	Stderr    io.Writer
	Stdin     io.Reader
	Stdout    io.Writer

	commandContext func(ctx context.Context, name string, args ...string) *exec.Cmd
}

func (c *Client) Command(args ...string) (*gitCommand, error) {
	preArgs := []string{}
	if c.GhPath != "" {
		preArgs = append(preArgs, "-c", "credential.helper=''")
		for _, host := range c.AuthHosts {
			preArgs = append(preArgs, "-c", fmt.Sprintf("credential.https://%s.helper=''", host))
		}
		credHelper := fmt.Sprintf("!%s auth git-credential", c.GhPath)
		preArgs = append(preArgs, "-c", fmt.Sprintf("credential.helper='%s'", credHelper))
	}
	if c.RepoDir != "" {
		preArgs = append(preArgs, "-C", c.RepoDir)
	}
	args = append(preArgs, args...)
	if c.commandContext == nil {
		c.commandContext = exec.CommandContext
	}
	if c.GitPath == "" {
		var err error
		c.GitPath, err = safeexec.LookPath("git")
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				programName := "git"
				if runtime.GOOS == "windows" {
					programName = "Git for Windows"
				}
				return nil, &NotInstalled{
					message: fmt.Sprintf("unable to find git executable in PATH; please install %s before retrying", programName),
					error:   err,
				}
			}
			return nil, err
		}
	}
	cmd := c.commandContext(context.Background(), c.GitPath, args...)
	//TODO: Will this swallow yubi key and other 2FA prompts?
	if c.GhPath != "" {
		cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0")
	}
	cmd.Stderr = c.Stderr
	//TODO: Somehow make it so stdin doesnt close and is useable after executing a command. Does this actually close stdin??
	cmd.Stdin = c.Stdin
	cmd.Stdout = c.Stdout
	return &gitCommand{cmd}, nil
}

func (c *Client) Remotes() (RemoteSet, error) {
	remoteArgs := []string{"remote", "-v"}
	remoteCmd, err := c.Command(remoteArgs...)
	if err != nil {
		return nil, err
	}
	remoteOutBuf, remoteErrBuf := bytes.Buffer{}, bytes.Buffer{}
	remoteCmd.Stderr, remoteCmd.Stdout = &remoteErrBuf, &remoteOutBuf
	if err := remoteCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run git: %s. error: %w", remoteErrBuf.String(), err)
	}

	configArgs := []string{"config", "--get-regexp", `^remote\..*\.gh-resolved$`}
	configCmd, err := c.Command(configArgs...)
	if err != nil {
		return nil, err
	}
	configOutBuf, configErrBuf := bytes.Buffer{}, bytes.Buffer{}
	configCmd.Stderr, configCmd.Stdout = &configErrBuf, &configOutBuf
	if err := configCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run git: %s. error: %w", configErrBuf.String(), err)
	}

	remotes := parseRemotes(outputLines(remoteOutBuf.Bytes()))
	populateResolvedRemotes(remotes, outputLines(configOutBuf.Bytes()))
	sort.Sort(remotes)
	return remotes, nil
}

func (c *Client) AddRemote(name, urlStr string, trackingBranches []string) (*Remote, error) {
	args := []string{"remote", "add"}
	for _, branch := range trackingBranches {
		args = append(args, "-t", branch)
	}
	args = append(args, "-f", name, urlStr)
	cmd, err := c.Command(args...)
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

func (c *Client) UpdateRemoteURL(name, url string) error {
	args := []string{"remote", "set-url", name, url}
	cmd, err := c.Command(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (c *Client) SetRemoteResolution(name, resolution string) error {
	args := []string{"config", "--add", fmt.Sprintf("remote.%s.gh-resolved", name), resolution}
	cmd, err := c.Command(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

// CurrentBranch reads the checked-out branch for the git repository.
func (c *Client) CurrentBranch() (string, error) {
	args := []string{"symbolic-ref", "--quiet", "HEAD"}
	cmd, err := c.Command(args...)
	if err != nil {
		return "", err
	}
	outBuf, errBuf := bytes.Buffer{}, bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	if err := cmd.Run(); err != nil {
		if errBuf.Len() == 0 {
			return "", ErrNotOnAnyBranch
		}
		return "", fmt.Errorf("failed to run git: %s. error: %w", errBuf.String(), err)
	}
	branch := firstLine(outBuf.Bytes())
	return strings.TrimPrefix(branch, "refs/heads/"), nil
}

// ShowRefs resolves fully-qualified refs to commit hashes.
func (c *Client) ShowRefs(ref ...string) ([]Ref, error) {
	args := append([]string{"show-ref", "--verify", "--"}, ref...)
	cmd, err := c.Command(args...)
	if err != nil {
		return nil, err
	}
	outBuf, errBuf := bytes.Buffer{}, bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run git: %s. error: %w", errBuf.String(), err)
	}
	var refs []Ref
	for _, line := range outputLines(outBuf.Bytes()) {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		refs = append(refs, Ref{
			Hash: parts[0],
			Name: parts[1],
		})
	}
	return refs, nil
}

func (c *Client) Config(name string) (string, error) {
	args := []string{"config", name}
	cmd, err := c.Command(args...)
	if err != nil {
		return "", err
	}
	outBuf, errBuf := bytes.Buffer{}, bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("unknown config key: %s", name)
	}
	return firstLine(outBuf.Bytes()), nil
}

func (c *Client) UncommittedChangeCount() (int, error) {
	args := []string{"status", "--porcelain"}
	cmd, err := c.Command(args...)
	if err != nil {
		return 0, err
	}
	outBuf, errBuf := bytes.Buffer{}, bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("failed to run git: %s. error: %w", errBuf.String(), err)
	}
	lines := strings.Split(outBuf.String(), "\n")
	count := 0
	for _, l := range lines {
		if l != "" {
			count++
		}
	}
	return count, nil
}

func (c *Client) Commits(baseRef, headRef string) ([]*Commit, error) {
	args := []string{"-c", "log.ShowSignature=false", "log", "--pretty=format:%H,%s", "--cherry", fmt.Sprintf("%s...%s", baseRef, headRef)}
	cmd, err := c.Command(args...)
	if err != nil {
		return nil, err
	}
	outBuf, errBuf := bytes.Buffer{}, bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run git: %s. error: %w", errBuf.String(), err)
	}
	commits := []*Commit{}
	sha := 0
	title := 1
	for _, line := range outputLines(outBuf.Bytes()) {
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

func (c *Client) lookupCommit(sha, format string) ([]byte, error) {
	args := []string{"-c", "log.ShowSignature=false", "show", "-s", "--pretty=format:" + format, sha}
	cmd, err := c.Command(args...)
	if err != nil {
		return nil, err
	}
	outBuf, errBuf := bytes.Buffer{}, bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run git: %s. error: %w", errBuf.String(), err)
	}
	return outBuf.Bytes(), nil
}

func (c *Client) LastCommit() (*Commit, error) {
	output, err := c.lookupCommit("HEAD", "%H,%s")
	if err != nil {
		return nil, err
	}
	idx := bytes.IndexByte(output, ',')
	return &Commit{
		Sha:   string(output[0:idx]),
		Title: strings.TrimSpace(string(output[idx+1:])),
	}, nil
}

func (c *Client) CommitBody(sha string) (string, error) {
	output, err := c.lookupCommit(sha, "%b")
	return string(output), err
}

// Push publishes a git ref to a remote and sets up upstream configuration.
func (c *Client) Push(remote string, ref string) error {
	args := []string{"push", "--set-upstream", remote, ref}
	cmd, err := c.Command(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

// ReadBranchConfig parses the `branch.BRANCH.(remote|merge)` part of git config.
func (c *Client) ReadBranchConfig(branch string) (cfg BranchConfig) {
	prefix := regexp.QuoteMeta(fmt.Sprintf("branch.%s.", branch))
	args := []string{"config", "--get-regexp", fmt.Sprintf("^%s(remote|merge)$", prefix)}
	cmd, err := c.Command(args...)
	if err != nil {
		return
	}
	outBuf, errBuf := bytes.Buffer{}, bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	if err := cmd.Run(); err != nil {
		return
	}
	for _, line := range outputLines(outBuf.Bytes()) {
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

func (c *Client) DeleteLocalBranch(branch string) error {
	args := []string{"branch", "-D", branch}
	cmd, err := c.Command(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (c *Client) HasLocalBranch(branch string) bool {
	args := []string{"rev-parse", "--verify", "refs/heads/" + branch}
	cmd, err := c.Command(args...)
	if err != nil {
		return false
	}
	err = cmd.Run()
	return err == nil
}

func (c *Client) CheckoutBranch(branch string) error {
	args := []string{"checkout", branch}
	cmd, err := c.Command(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (c *Client) CheckoutNewBranch(remoteName, branch string) error {
	track := fmt.Sprintf("%s/%s", remoteName, branch)
	args := []string{"checkout", "-b", branch, "--track", track}
	cmd, err := c.Command(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (c *Client) Pull(remote, branch string) error {
	args := []string{"pull", "--ff-only", remote, branch}
	cmd, err := c.Command(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (c *Client) Clone(cloneURL string, args []string) (target string, err error) {
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

	cmd, err := c.Command(cloneArgs...)
	if err != nil {
		return "", err
	}

	err = cmd.Run()
	return
}

// ToplevelDir returns the top-level directory path of the current repository.
func (c *Client) ToplevelDir() (string, error) {
	args := []string{"rev-parse", "--show-toplevel"}
	cmd, err := c.Command(args...)
	if err != nil {
		return "", err
	}
	outBuf, errBuf := bytes.Buffer{}, bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run git: %s. error: %w", errBuf.String(), err)
	}
	return firstLine(outBuf.Bytes()), nil
}

func (c *Client) GitDir() (string, error) {
	args := []string{"rev-parse", "--git-dir"}
	cmd, err := c.Command(args...)
	if err != nil {
		return "", err
	}
	outBuf, errBuf := bytes.Buffer{}, bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run git: %s. error: %w", errBuf.String(), err)
	}
	return firstLine(outBuf.Bytes()), nil
}

// Show current directory relative to the top-level directory of repository.
func (c *Client) PathFromRoot() string {
	args := []string{"rev-parse", "--show-prefix"}
	cmd, err := c.Command(args...)
	if err != nil {
		return ""
	}
	outBuf, errBuf := bytes.Buffer{}, bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	if err := cmd.Run(); err != nil {
		return ""
	}
	if path := firstLine(outBuf.Bytes()); path != "" {
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
