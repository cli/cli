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
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/safeexec"
)

// MergeBaseConfig is the configuration setting to keep track of the PR target branch.
const MergeBaseConfig = "gh-merge-base"

var remoteRE = regexp.MustCompile(`(.+)\s+(.+)\s+\((push|fetch)\)`)

// This regexp exists to match lines of the following form:
// 6a6872b918c601a0e730710ad8473938a7516d30\u0000title 1\u0000Body 1\u0000\n
// 7a6872b918c601a0e730710ad8473938a7516d31\u0000title 2\u0000Body 2\u0000
//
// This is the format we use when collecting commit information,
// with null bytes as separators. Using null bytes this way allows for us
// to easily maintain newlines that might be in the body.
//
// The ?m modifier is the multi-line modifier, meaning that ^ and $
// match the beginning and end of lines, respectively.
//
// The [\S\s] matches any whitespace or non-whitespace character,
// which is different from .* because it allows for newlines as well.
//
// The ? following .* and [\S\s] is a lazy modifier, meaning that it will
// match as few characters as possible while still satisfying the rest of the regexp.
// This is important because it allows us to match the first null byte after the title and body,
// rather than the last null byte in the entire string.
var commitLogRE = regexp.MustCompile(`(?m)^[0-9a-fA-F]{7,40}\x00.*?\x00[\S\s]*?\x00$`)

type errWithExitCode interface {
	ExitCode() int
}

type Client struct {
	GhPath  string
	RepoDir string
	GitPath string
	Stderr  io.Writer
	Stdin   io.Reader
	Stdout  io.Writer

	commandContext commandCtx
	mu             sync.Mutex
}

func (c *Client) Copy() *Client {
	return &Client{
		GhPath:  c.GhPath,
		RepoDir: c.RepoDir,
		GitPath: c.GitPath,
		Stderr:  c.Stderr,
		Stdin:   c.Stdin,
		Stdout:  c.Stdout,

		commandContext: c.commandContext,
	}
}

func (c *Client) Command(ctx context.Context, args ...string) (*Command, error) {
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
	return &Command{cmd}, nil
}

// CredentialPattern is used to inform AuthenticatedCommand which patterns Git should match
// against when trying to find credentials. It is a little over-engineered as a type because we
// want AuthenticatedCommand to have a clear compilation error when this is not provided,
// as opposed to using a string which might compile with `client.AuthenticatedCommand(ctx, "fetch")`.
//
// It is only usable when constructed by another function in the package because the empty pattern,
// without allMatching set to true, will result in an error in AuthenticatedCommand.
//
// Callers can currently opt-in to an slightly less secure mode for backwards compatibility by using
// AllMatchingCredentialsPattern.
type CredentialPattern struct {
	allMatching bool // should only be constructable via AllMatchingCredentialsPattern
	pattern     string
}

// AllMatchingCredentialsPattern allows for setting gh as credential helper for all hosts.
// However, we should endeavour to remove it as it's less secure.
var AllMatchingCredentialsPattern = CredentialPattern{allMatching: true, pattern: ""}
var disallowedCredentialPattern = CredentialPattern{allMatching: false, pattern: ""}

// CredentialPatternFromGitURL takes a git remote URL e.g. "https://github.com/cli/cli.git" or
// "git@github.com:cli/cli.git" and returns the credential pattern that should be used for it.
func CredentialPatternFromGitURL(gitURL string) (CredentialPattern, error) {
	normalizedURL, err := ParseURL(gitURL)
	if err != nil {
		return CredentialPattern{}, fmt.Errorf("failed to parse remote URL: %w", err)
	}
	return CredentialPatternFromHost(normalizedURL.Host), nil
}

// CredentialPatternFromHost expects host to be in the form "github.com" and returns
// the credential pattern that should be used for it.
// It does not perform any canonicalisation e.g. "api.github.com" will not work as expected.
func CredentialPatternFromHost(host string) CredentialPattern {
	return CredentialPattern{
		pattern: strings.TrimSuffix(ghinstance.HostPrefix(host), "/"),
	}
}

// AuthenticatedCommand is a wrapper around Command that included configuration to use gh
// as the credential helper for git.
func (c *Client) AuthenticatedCommand(ctx context.Context, credentialPattern CredentialPattern, args ...string) (*Command, error) {
	if c.GhPath == "" {
		// Assumes that gh is in PATH.
		c.GhPath = "gh"
	}
	credHelper := fmt.Sprintf("!%q auth git-credential", c.GhPath)

	var preArgs []string
	if credentialPattern == disallowedCredentialPattern {
		return nil, fmt.Errorf("empty credential pattern is not allowed unless provided explicitly")
	} else if credentialPattern == AllMatchingCredentialsPattern {
		preArgs = []string{"-c", "credential.helper="}
		preArgs = append(preArgs, "-c", fmt.Sprintf("credential.helper=%s", credHelper))
	} else {
		preArgs = []string{"-c", fmt.Sprintf("credential.%s.helper=", credentialPattern.pattern)}
		preArgs = append(preArgs, "-c", fmt.Sprintf("credential.%s.helper=%s", credentialPattern.pattern, credHelper))
	}

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
		return nil, remoteErr
	}

	configArgs := []string{"config", "--get-regexp", `^remote\..*\.gh-resolved$`}
	configCmd, err := c.Command(ctx, configArgs...)
	if err != nil {
		return nil, err
	}
	configOut, configErr := configCmd.Output()
	if configErr != nil {
		// Ignore exit code 1 as it means there are no resolved remotes.
		var gitErr *GitError
		if ok := errors.As(configErr, &gitErr); ok && gitErr.ExitCode != 1 {
			return nil, gitErr
		}
	}

	remotes := parseRemotes(outputLines(remoteOut))
	populateResolvedRemotes(remotes, outputLines(configOut))
	sort.Sort(remotes)
	return remotes, nil
}

func (c *Client) UpdateRemoteURL(ctx context.Context, name, url string) error {
	args := []string{"remote", "set-url", name, url}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	_, err = cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) SetRemoteResolution(ctx context.Context, name, resolution string) error {
	args := []string{"config", "--add", fmt.Sprintf("remote.%s.gh-resolved", name), resolution}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	_, err = cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

// CurrentBranch reads the checked-out branch for the git repository.
func (c *Client) CurrentBranch(ctx context.Context) (string, error) {
	args := []string{"symbolic-ref", "--quiet", "HEAD"}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return "", err
	}
	out, err := cmd.Output()
	if err != nil {
		var gitErr *GitError
		if ok := errors.As(err, &gitErr); ok && len(gitErr.Stderr) == 0 {
			gitErr.err = ErrNotOnAnyBranch
			gitErr.Stderr = "not on any branch"
			return "", gitErr
		}
		return "", err
	}
	branch := firstLine(out)
	return strings.TrimPrefix(branch, "refs/heads/"), nil
}

// ShowRefs resolves fully-qualified refs to commit hashes.
func (c *Client) ShowRefs(ctx context.Context, refs []string) ([]Ref, error) {
	args := append([]string{"show-ref", "--verify", "--"}, refs...)
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return nil, err
	}
	// This functionality relies on parsing output from the git command despite
	// an error status being returned from git.
	out, err := cmd.Output()
	var verified []Ref
	for _, line := range outputLines(out) {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		verified = append(verified, Ref{
			Hash: parts[0],
			Name: parts[1],
		})
	}
	return verified, err
}

func (c *Client) Config(ctx context.Context, name string) (string, error) {
	args := []string{"config", name}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return "", err
	}
	out, err := cmd.Output()
	if err != nil {
		var gitErr *GitError
		if ok := errors.As(err, &gitErr); ok && gitErr.ExitCode == 1 {
			gitErr.Stderr = fmt.Sprintf("unknown config key %s", name)
			return "", gitErr
		}
		return "", err
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
		return 0, err
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
	// The formatting directive %x00 indicates that git should include the null byte as a separator.
	// We use this because it is not a valid character to include in a commit message. Previously,
	// commas were used here but when we Split on them, we would get incorrect results if commit titles
	// happened to contain them.
	// https://git-scm.com/docs/pretty-formats#Documentation/pretty-formats.txt-emx00em
	args := []string{"-c", "log.ShowSignature=false", "log", "--pretty=format:%H%x00%s%x00%b%x00", "--cherry", fmt.Sprintf("%s...%s", baseRef, headRef)}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return nil, err
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	commits := []*Commit{}
	commitLogs := commitLogRE.FindAllString(string(out), -1)
	for _, commitLog := range commitLogs {
		//  Each line looks like this:
		//  6a6872b918c601a0e730710ad8473938a7516d30\u0000title 1\u0000Body 1\u0000\n

		//  Or with an optional body:
		//  6a6872b918c601a0e730710ad8473938a7516d30\u0000title 1\u0000\u0000\n

		//  Therefore after splitting we will have:
		//  ["6a6872b918c601a0e730710ad8473938a7516d30", "title 1", "Body 1", ""]

		//  Or with an optional body:
		//  ["6a6872b918c601a0e730710ad8473938a7516d30", "title 1", "", ""]
		commitLogParts := strings.Split(commitLog, "\u0000")
		commits = append(commits, &Commit{
			Sha:   commitLogParts[0],
			Title: commitLogParts[1],
			Body:  commitLogParts[2],
		})
	}

	if len(commits) == 0 {
		return nil, fmt.Errorf("could not find any commits between %s and %s", baseRef, headRef)
	}

	return commits, nil
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

func (c *Client) lookupCommit(ctx context.Context, sha, format string) ([]byte, error) {
	args := []string{"-c", "log.ShowSignature=false", "show", "-s", "--pretty=format:" + format, sha}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return nil, err
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ReadBranchConfig parses the `branch.BRANCH.(remote|merge|gh-merge-base)` part of git config.
// If no branch config is found or there is an error in the command, it returns an empty BranchConfig.
// Downstream consumers of ReadBranchConfig should consider the behavior they desire if this errors,
// as an empty config is not necessarily breaking.
func (c *Client) ReadBranchConfig(ctx context.Context, branch string) (BranchConfig, error) {

	prefix := regexp.QuoteMeta(fmt.Sprintf("branch.%s.", branch))
	args := []string{"config", "--get-regexp", fmt.Sprintf("^%s(remote|merge|%s)$", prefix, MergeBaseConfig)}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return BranchConfig{}, err
	}

	out, err := cmd.Output()
	if err != nil {
		// This is the error we expect if the git command does not run successfully.
		// If the ExitCode is 1, then we just didn't find any config for the branch.
		var gitError *GitError
		if ok := errors.As(err, &gitError); ok && gitError.ExitCode != 1 {
			return BranchConfig{}, err
		}
		return BranchConfig{}, nil
	}

	return parseBranchConfig(outputLines(out)), nil
}

func parseBranchConfig(configLines []string) BranchConfig {
	var cfg BranchConfig

	for _, line := range configLines {
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
		case MergeBaseConfig:
			cfg.MergeBase = parts[1]
		}
	}
	return cfg
}

// SetBranchConfig sets the named value on the given branch.
func (c *Client) SetBranchConfig(ctx context.Context, branch, name, value string) error {
	name = fmt.Sprintf("branch.%s.%s", branch, name)
	args := []string{"config", name, value}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	// No output expected but check for any printed git error.
	_, err = cmd.Output()
	return err
}

func (c *Client) DeleteLocalTag(ctx context.Context, tag string) error {
	args := []string{"tag", "-d", tag}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	_, err = cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) DeleteLocalBranch(ctx context.Context, branch string) error {
	args := []string{"branch", "-D", branch}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	_, err = cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) CheckoutBranch(ctx context.Context, branch string) error {
	args := []string{"checkout", branch}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	_, err = cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) CheckoutNewBranch(ctx context.Context, remoteName, branch string) error {
	track := fmt.Sprintf("%s/%s", remoteName, branch)
	args := []string{"checkout", "-b", branch, "--track", track}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	_, err = cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) HasLocalBranch(ctx context.Context, branch string) bool {
	_, err := c.revParse(ctx, "--verify", "refs/heads/"+branch)
	return err == nil
}

func (c *Client) TrackingBranchNames(ctx context.Context, prefix string) []string {
	args := []string{"branch", "-r", "--format", "%(refname:strip=3)"}
	if prefix != "" {
		args = append(args, "--list", fmt.Sprintf("*/%s*", escapeGlob(prefix)))
	}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return nil
	}
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	return strings.Split(string(output), "\n")
}

// ToplevelDir returns the top-level directory path of the current repository.
func (c *Client) ToplevelDir(ctx context.Context) (string, error) {
	out, err := c.revParse(ctx, "--show-toplevel")
	if err != nil {
		return "", err
	}
	return firstLine(out), nil
}

func (c *Client) GitDir(ctx context.Context) (string, error) {
	out, err := c.revParse(ctx, "--git-dir")
	if err != nil {
		return "", err
	}
	return firstLine(out), nil
}

// Show current directory relative to the top-level directory of repository.
func (c *Client) PathFromRoot(ctx context.Context) string {
	out, err := c.revParse(ctx, "--show-prefix")
	if err != nil {
		return ""
	}
	if path := firstLine(out); path != "" {
		return path[:len(path)-1]
	}
	return ""
}

func (c *Client) revParse(ctx context.Context, args ...string) ([]byte, error) {
	args = append([]string{"rev-parse"}, args...)
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return nil, err
	}
	return cmd.Output()
}

func (c *Client) IsLocalGitRepo(ctx context.Context) (bool, error) {
	_, err := c.GitDir(ctx)
	if err != nil {
		var execError errWithExitCode
		if errors.As(err, &execError) && execError.ExitCode() == 128 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *Client) UnsetRemoteResolution(ctx context.Context, name string) error {
	args := []string{"config", "--unset", fmt.Sprintf("remote.%s.gh-resolved", name)}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	_, err = cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) SetRemoteBranches(ctx context.Context, remote string, refspec string) error {
	args := []string{"remote", "set-branches", remote, refspec}
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return err
	}
	_, err = cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) AddRemote(ctx context.Context, name, urlStr string, trackingBranches []string) (*Remote, error) {
	args := []string{"remote", "add"}
	for _, branch := range trackingBranches {
		args = append(args, "-t", branch)
	}
	args = append(args, name, urlStr)
	cmd, err := c.Command(ctx, args...)
	if err != nil {
		return nil, err
	}
	if _, err := cmd.Output(); err != nil {
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

// Below are commands that make network calls and need authentication credentials supplied from gh.

func (c *Client) Fetch(ctx context.Context, remote string, refspec string, mods ...CommandModifier) error {
	args := []string{"fetch", remote}
	if refspec != "" {
		args = append(args, refspec)
	}
	cmd, err := c.AuthenticatedCommand(ctx, AllMatchingCredentialsPattern, args...)
	if err != nil {
		return err
	}
	for _, mod := range mods {
		mod(cmd)
	}
	return cmd.Run()
}

func (c *Client) Pull(ctx context.Context, remote, branch string, mods ...CommandModifier) error {
	args := []string{"pull", "--ff-only"}
	if remote != "" && branch != "" {
		args = append(args, remote, branch)
	}
	cmd, err := c.AuthenticatedCommand(ctx, AllMatchingCredentialsPattern, args...)
	if err != nil {
		return err
	}
	for _, mod := range mods {
		mod(cmd)
	}
	return cmd.Run()
}

func (c *Client) Push(ctx context.Context, remote string, ref string, mods ...CommandModifier) error {
	args := []string{"push", "--set-upstream", remote, ref}
	cmd, err := c.AuthenticatedCommand(ctx, AllMatchingCredentialsPattern, args...)
	if err != nil {
		return err
	}
	for _, mod := range mods {
		mod(cmd)
	}
	return cmd.Run()
}

func (c *Client) Clone(ctx context.Context, cloneURL string, args []string, mods ...CommandModifier) (string, error) {
	// Note that even if this is an SSH clone URL, we are setting the pattern anyway.
	// We could write some code to prevent this, but it also doesn't seem harmful.
	pattern, err := CredentialPatternFromGitURL(cloneURL)
	if err != nil {
		return "", err
	}

	cloneArgs, target := parseCloneArgs(args)
	cloneArgs = append(cloneArgs, cloneURL)
	// If the args contain an explicit target, pass it to clone otherwise,
	// parse the URL to determine where git cloned it to so we can return it.
	if target != "" {
		cloneArgs = append(cloneArgs, target)
	} else {
		target = path.Base(strings.TrimSuffix(cloneURL, ".git"))

		if slices.Contains(cloneArgs, "--bare") {
			target += ".git"
		}
	}
	cloneArgs = append([]string{"clone"}, cloneArgs...)
	cmd, err := c.AuthenticatedCommand(ctx, pattern, cloneArgs...)
	if err != nil {
		return "", err
	}
	for _, mod := range mods {
		mod(cmd)
	}
	err = cmd.Run()
	if err != nil {
		return "", err
	}
	return target, nil
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

var globReplacer = strings.NewReplacer(
	"*", `\*`,
	"?", `\?`,
	"[", `\[`,
	"]", `\]`,
	"{", `\{`,
	"}", `\}`,
)

func escapeGlob(p string) string {
	return globReplacer.Replace(p)
}
