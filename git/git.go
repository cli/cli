package git

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/cli/cli/internal/run"
)

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
	showRef := exec.Command("git", args...)
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
	refCmd := GitCommand("symbolic-ref", "--quiet", "--short", "HEAD")

	output, err := run.PrepareCmd(refCmd).Output()
	if err == nil {
		// Found the branch name
		return firstLine(output), nil
	}

	var cmdErr *run.CmdError
	if errors.As(err, &cmdErr) {
		if cmdErr.Stderr.Len() == 0 {
			// Detached head
			return "", errors.New("git: not on any branch")
		}
	}

	// Unknown error
	return "", err
}

func listRemotes() ([]string, error) {
	remoteCmd := exec.Command("git", "remote", "-v")
	output, err := run.PrepareCmd(remoteCmd).Output()
	return outputLines(output), err
}

func Config(name string) (string, error) {
	configCmd := exec.Command("git", "config", name)
	output, err := run.PrepareCmd(configCmd).Output()
	if err != nil {
		return "", fmt.Errorf("unknown config key: %s", name)
	}

	return firstLine(output), nil

}

var GitCommand = func(args ...string) *exec.Cmd {
	return exec.Command("git", args...)
}

func UncommittedChangeCount() (int, error) {
	statusCmd := GitCommand("status", "--porcelain")
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
	logCmd := GitCommand(
		"-c", "log.ShowSignature=false",
		"log", "--pretty=format:%H,%s",
		"--cherry", fmt.Sprintf("%s...%s", baseRef, headRef))
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

func CommitBody(sha string) (string, error) {
	showCmd := GitCommand("-c", "log.ShowSignature=false", "show", "-s", "--pretty=format:%b", sha)
	output, err := run.PrepareCmd(showCmd).Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// Push publishes a git ref to a remote and sets up upstream configuration
func Push(remote string, ref string) error {
	pushCmd := GitCommand("push", "--set-upstream", remote, ref)
	pushCmd.Stdout = os.Stdout
	pushCmd.Stderr = os.Stderr
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
	configCmd := GitCommand("config", "--get-regexp", fmt.Sprintf("^%s(remote|merge)$", prefix))
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

func isFilesystemPath(p string) bool {
	return p == "." || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "/")
}

// ToplevelDir returns the top-level directory path of the current repository
func ToplevelDir() (string, error) {
	showCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := run.PrepareCmd(showCmd).Output()
	return firstLine(output), err

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
