package git

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/github/gh-cli/utils"
)

func Dir() (string, error) {
	dirCmd := exec.Command("git", "rev-parse", "-q", "--git-dir")
	output, err := utils.PrepareCmd(dirCmd).Output()
	if err != nil {
		return "", fmt.Errorf("Not a git repository (or any of the parent directories): .git")
	}

	gitDir := firstLine(output)
	if !filepath.IsAbs(gitDir) {
		gitDir, err = filepath.Abs(gitDir)
		if err != nil {
			return "", err
		}
		gitDir = filepath.Clean(gitDir)
	}

	return gitDir, nil
}

func WorkdirName() (string, error) {
	toplevelCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	toplevelCmd.Stderr = nil
	output, err := utils.PrepareCmd(toplevelCmd).Output()
	dir := firstLine(output)
	if dir == "" {
		return "", fmt.Errorf("unable to determine git working directory")
	}
	return dir, err
}

func HasFile(segments ...string) bool {
	// The blessed way to resolve paths within git dir since Git 2.5.0
	pathCmd := exec.Command("git", "rev-parse", "-q", "--git-path", filepath.Join(segments...))
	if output, err := utils.PrepareCmd(pathCmd).Output(); err == nil {
		if lines := outputLines(output); len(lines) == 1 {
			if _, err := os.Stat(lines[0]); err == nil {
				return true
			}
		}
	}

	// Fallback for older git versions
	dir, err := Dir()
	if err != nil {
		return false
	}

	s := []string{dir}
	s = append(s, segments...)
	path := filepath.Join(s...)
	if _, err := os.Stat(path); err == nil {
		return true
	}

	return false
}

func BranchAtRef(paths ...string) (name string, err error) {
	dir, err := Dir()
	if err != nil {
		return
	}

	segments := []string{dir}
	segments = append(segments, paths...)
	path := filepath.Join(segments...)
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}

	n := string(b)
	refPrefix := "ref: "
	if strings.HasPrefix(n, refPrefix) {
		name = strings.TrimPrefix(n, refPrefix)
		name = strings.TrimSpace(name)
	} else {
		err = fmt.Errorf("No branch info in %s: %s", path, n)
	}

	return
}

func Editor() (string, error) {
	varCmd := exec.Command("git", "var", "GIT_EDITOR")
	output, err := utils.PrepareCmd(varCmd).Output()
	if err != nil {
		return "", fmt.Errorf("Can't load git var: GIT_EDITOR")
	}

	return os.ExpandEnv(firstLine(output)), nil
}

func Head() (string, error) {
	return BranchAtRef("HEAD")
}

func SymbolicFullName(name string) (string, error) {
	parseCmd := exec.Command("git", "rev-parse", "--symbolic-full-name", name)
	output, err := utils.PrepareCmd(parseCmd).Output()
	if err != nil {
		return "", fmt.Errorf("Unknown revision or path not in the working tree: %s", name)
	}

	return firstLine(output), nil
}

func CommentChar(text string) (string, error) {
	char, err := Config("core.commentchar")
	if err != nil {
		return "#", nil
	} else if char == "auto" {
		lines := strings.Split(text, "\n")
		commentCharCandidates := strings.Split("#;@!$%^&|:", "")
	candidateLoop:
		for _, candidate := range commentCharCandidates {
			for _, line := range lines {
				if strings.HasPrefix(line, candidate) {
					continue candidateLoop
				}
			}
			return candidate, nil
		}
		return "", fmt.Errorf("unable to select a comment character that is not used in the current message")
	} else {
		return char, nil
	}
}

func Show(sha string) (string, error) {
	cmd := exec.Command("git", "-c", "log.showSignature=false", "show", "-s", "--format=%s%n%+b", sha)
	output, err := utils.PrepareCmd(cmd).Output()
	return strings.TrimSpace(string(output)), err
}

func Log(sha1, sha2 string) (string, error) {
	shaRange := fmt.Sprintf("%s...%s", sha1, sha2)
	cmd := exec.Command(
		"-c", "log.showSignature=false", "log", "--no-color",
		"--format=%h (%aN, %ar)%n%w(78,3,3)%s%n%+b",
		"--cherry", shaRange)
	outputs, err := utils.PrepareCmd(cmd).Output()
	if err != nil {
		return "", fmt.Errorf("Can't load git log %s..%s", sha1, sha2)
	}

	return string(outputs), nil
}

func listRemotes() ([]string, error) {
	remoteCmd := exec.Command("git", "remote", "-v")
	output, err := utils.PrepareCmd(remoteCmd).Output()
	return outputLines(output), err
}

func Config(name string) (string, error) {
	configCmd := exec.Command("git", "config", name)
	output, err := utils.PrepareCmd(configCmd).Output()
	if err != nil {
		return "", fmt.Errorf("unknown config key: %s", name)
	}

	return firstLine(output), nil

}

func ConfigAll(name string) ([]string, error) {
	mode := "--get-all"
	if strings.Contains(name, "*") {
		mode = "--get-regexp"
	}

	configCmd := exec.Command("git", "config", mode, name)
	output, err := utils.PrepareCmd(configCmd).Output()
	if err != nil {
		return nil, fmt.Errorf("Unknown config %s", name)
	}
	return outputLines(output), nil
}

func LocalBranches() ([]string, error) {
	branchesCmd := exec.Command("git", "branch", "--list")
	output, err := utils.PrepareCmd(branchesCmd).Output()
	if err != nil {
		return nil, err
	}

	branches := []string{}
	for _, branch := range outputLines(output) {
		branches = append(branches, branch[2:])
	}
	return branches, nil
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
