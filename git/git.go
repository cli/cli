package git

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"strings"

	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/safeexec"
)

// ErrNotOnAnyBranch indicates that the user is in detached HEAD state
var ErrNotOnAnyBranch = errors.New("git: not on any branch")

// Ref represents a git commit reference
type Ref struct {
	Hash string
	Name string
}

// TrackingRef represents a ref for a remote tracking branch
type TrackingRef struct {
	RemoteName string
	BranchName string
}

func (r TrackingRef) String() string {
	return "refs/remotes/" + r.RemoteName + "/" + r.BranchName
}

// ShowRefs resolves fully-qualified refs to commit hashes
func ShowRefs(ref ...string) ([]Ref, error) {
	args := append([]string{"show-ref", "--verify", "--"}, ref...)
	showRef, err := GitCommand(args...)
	if err != nil {
		return nil, err
	}
	output, err := run.PrepareCmd(showRef).Output()

	var refs []Ref
	for _, line := range outputLines(output) {
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

// CurrentBranch reads the checked-out branch for the git repository
func CurrentBranch() (string, error) {
	refCmd, err := GitCommand("symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return "", err
	}

	stderr := bytes.Buffer{}
	refCmd.Stderr = &stderr

	output, err := run.PrepareCmd(refCmd).Output()
	if err == nil {
		// Found the branch name
		return getBranchShortName(output), nil
	}

	if stderr.Len() == 0 {
		// Detached head
		return "", ErrNotOnAnyBranch
	}

	return "", fmt.Errorf("%sgit: %s", stderr.String(), err)
}

func listRemotesForPath(path string) ([]string, error) {
	remoteCmd, err := GitCommand("-C", path, "remote", "-v")
	if err != nil {
		return nil, err
	}
	output, err := run.PrepareCmd(remoteCmd).Output()
	return outputLines(output), err
}

func listRemotes() ([]string, error) {
	remoteCmd, err := GitCommand("remote", "-v")
	if err != nil {
		return nil, err
	}
	output, err := run.PrepareCmd(remoteCmd).Output()
	return outputLines(output), err
}

func Config(name string) (string, error) {
	configCmd, err := GitCommand("config", name)
	if err != nil {
		return "", err
	}
	output, err := run.PrepareCmd(configCmd).Output()
	if err != nil {
		return "", fmt.Errorf("unknown config key: %s", name)
	}

	return firstLine(output), nil

}

type NotInstalled struct {
	message string
	error
}

func (e *NotInstalled) Error() string {
	return e.message
}

func GitCommand(args ...string) (*exec.Cmd, error) {
	gitExe, err := safeexec.LookPath("git")
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
	return exec.Command(gitExe, args...), nil
}

func UncommittedChangeCount() (int, error) {
	statusCmd, err := GitCommand("status", "--porcelain")
	if err != nil {
		return 0, err
	}
	output, err := run.PrepareCmd(statusCmd).Output()
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(output), "\n")

	count := 0

	for _, l := range lines {
		if l != "" {
			count++
		}
	}

	return count, nil
}

type Commit struct {
	Sha   string
	Title string
}

func Commits(baseRef, headRef string) ([]*Commit, error) {
	logCmd, err := GitCommand(
		"-c", "log.ShowSignature=false",
		"log", "--pretty=format:%H,%s",
		"--cherry", fmt.Sprintf("%s...%s", baseRef, headRef))
	if err != nil {
		return nil, err
	}
	output, err := run.PrepareCmd(logCmd).Output()
	if err != nil {
		return []*Commit{}, err
	}

	commits := []*Commit{}
	sha := 0
	title := 1
	for _, line := range outputLines(output) {
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
		return commits, fmt.Errorf("could not find any commits between %s and %s", baseRef, headRef)
	}

	return commits, nil
}

func lookupCommit(sha, format string) ([]byte, error) {
	logCmd, err := GitCommand("-c", "log.ShowSignature=false", "show", "-s", "--pretty=format:"+format, sha)
	if err != nil {
		return nil, err
	}
	return run.PrepareCmd(logCmd).Output()
}

func LastCommit() (*Commit, error) {
	output, err := lookupCommit("HEAD", "%H,%s")
	if err != nil {
		return nil, err
	}

	idx := bytes.IndexByte(output, ',')
	return &Commit{
		Sha:   string(output[0:idx]),
		Title: strings.TrimSpace(string(output[idx+1:])),
	}, nil
}

func CommitBody(sha string) (string, error) {
	output, err := lookupCommit(sha, "%b")
	return string(output), err
}

// Push publishes a git ref to a remote and sets up upstream configuration
func Push(remote string, ref string, cmdOut, cmdErr io.Writer) error {
	pushCmd, err := GitCommand("push", "--set-upstream", remote, ref)
	if err != nil {
		return err
	}
	pushCmd.Stdout = cmdOut
	pushCmd.Stderr = cmdErr
	return run.PrepareCmd(pushCmd).Run()
}

type BranchConfig struct {
	RemoteName string
	RemoteURL  *url.URL
	MergeRef   string
}

// ReadBranchConfig parses the `branch.BRANCH.(remote|merge)` part of git config
func ReadBranchConfig(branch string) (cfg BranchConfig) {
	prefix := regexp.QuoteMeta(fmt.Sprintf("branch.%s.", branch))
	configCmd, err := GitCommand("config", "--get-regexp", fmt.Sprintf("^%s(remote|merge)$", prefix))
	if err != nil {
		return
	}
	output, err := run.PrepareCmd(configCmd).Output()
	if err != nil {
		return
	}
	for _, line := range outputLines(output) {
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

func DeleteLocalBranch(branch string) error {
	branchCmd, err := GitCommand("branch", "-D", branch)
	if err != nil {
		return err
	}
	return run.PrepareCmd(branchCmd).Run()
}

func HasLocalBranch(branch string) bool {
	configCmd, err := GitCommand("rev-parse", "--verify", "refs/heads/"+branch)
	if err != nil {
		return false
	}
	_, err = run.PrepareCmd(configCmd).Output()
	return err == nil
}

func CheckoutBranch(branch string) error {
	configCmd, err := GitCommand("checkout", branch)
	if err != nil {
		return err
	}
	return run.PrepareCmd(configCmd).Run()
}

// pull changes from remote branch without version history
func Pull(remote, branch string) error {
	pullCmd, err := GitCommand("pull", "--ff-only", remote, branch)
	if err != nil {
		return err
	}

	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	pullCmd.Stdin = os.Stdin
	return run.PrepareCmd(pullCmd).Run()
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

func RunClone(cloneURL string, args []string) (target string, err error) {
	cloneArgs, target := parseCloneArgs(args)

	cloneArgs = append(cloneArgs, cloneURL)

	// If the args contain an explicit target, pass it to clone
	//    otherwise, parse the URL to determine where git cloned it to so we can return it
	if target != "" {
		cloneArgs = append(cloneArgs, target)
	} else {
		target = path.Base(strings.TrimSuffix(cloneURL, ".git"))
	}

	cloneArgs = append([]string{"clone"}, cloneArgs...)

	cloneCmd, err := GitCommand(cloneArgs...)
	if err != nil {
		return "", err
	}
	cloneCmd.Stdin = os.Stdin
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr

	err = run.PrepareCmd(cloneCmd).Run()
	return
}

func AddUpstreamRemote(upstreamURL, cloneDir string, branches []string) error {
	args := []string{"-C", cloneDir, "remote", "add"}
	for _, branch := range branches {
		args = append(args, "-t", branch)
	}
	args = append(args, "-f", "upstream", upstreamURL)
	cloneCmd, err := GitCommand(args...)
	if err != nil {
		return err
	}
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr
	return run.PrepareCmd(cloneCmd).Run()
}

func isFilesystemPath(p string) bool {
	return p == "." || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "/")
}

// ToplevelDir returns the top-level directory path of the current repository
func ToplevelDir() (string, error) {
	showCmd, err := GitCommand("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	output, err := run.PrepareCmd(showCmd).Output()
	return firstLine(output), err

}

// ToplevelDirFromPath returns the top-level given path of the current repository
func GetDirFromPath(p string) (string, error) {
	showCmd, err := GitCommand("-C", p, "rev-parse", "--git-dir")
	if err != nil {
		return "", err
	}
	output, err := run.PrepareCmd(showCmd).Output()
	return firstLine(output), err
}

func PathFromRepoRoot() string {
	showCmd, err := GitCommand("rev-parse", "--show-prefix")
	if err != nil {
		return ""
	}
	output, err := run.PrepareCmd(showCmd).Output()
	if err != nil {
		return ""
	}
	if path := firstLine(output); path != "" {
		return path[:len(path)-1]
	}
	return ""
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

func getBranchShortName(output []byte) string {
	branch := firstLine(output)
	return strings.TrimPrefix(branch, "refs/heads/")
}
