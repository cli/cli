package git

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var GlobalFlags []string

func Version() (string, error) {
	versionCmd := exec.Command("git", "version")
	output, err := versionCmd.Output()
	if err != nil {
		return "", fmt.Errorf("error running git version: %s", err)
	}
	return firstLine(output), nil
}

var cachedDir string

func Dir() (string, error) {
	if cachedDir != "" {
		return cachedDir, nil
	}

	dirCmd := exec.Command("git", "rev-parse", "-q", "--git-dir")
	dirCmd.Stderr = nil
	output, err := dirCmd.Output()
	if err != nil {
		return "", fmt.Errorf("Not a git repository (or any of the parent directories): .git")
	}

	var chdir string
	for i, flag := range GlobalFlags {
		if flag == "-C" {
			dir := GlobalFlags[i+1]
			if filepath.IsAbs(dir) {
				chdir = dir
			} else {
				chdir = filepath.Join(chdir, dir)
			}
		}
	}

	gitDir := firstLine(output)

	if !filepath.IsAbs(gitDir) {
		if chdir != "" {
			gitDir = filepath.Join(chdir, gitDir)
		}

		gitDir, err = filepath.Abs(gitDir)
		if err != nil {
			return "", err
		}

		gitDir = filepath.Clean(gitDir)
	}

	cachedDir = gitDir
	return gitDir, nil
}

func WorkdirName() (string, error) {
	toplevelCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	toplevelCmd.Stderr = nil
	output, err := toplevelCmd.Output()
	dir := firstLine(output)
	if dir == "" {
		return "", fmt.Errorf("unable to determine git working directory")
	}
	return dir, err
}

func HasFile(segments ...string) bool {
	// The blessed way to resolve paths within git dir since Git 2.5.0
	pathCmd := exec.Command("git", "rev-parse", "-q", "--git-path", filepath.Join(segments...))
	pathCmd.Stderr = nil
	if output, err := pathCmd.Output(); err == nil {
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
	varCmd.Stderr = nil
	output, err := varCmd.Output()
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
	parseCmd.Stderr = nil
	output, err := parseCmd.Output()
	if err != nil {
		return "", fmt.Errorf("Unknown revision or path not in the working tree: %s", name)
	}

	return firstLine(output), nil
}

func Ref(ref string) (string, error) {
	parseCmd := exec.Command("git", "rev-parse", "-q", ref)
	parseCmd.Stderr = nil
	output, err := parseCmd.Output()
	if err != nil {
		return "", fmt.Errorf("Unknown revision or path not in the working tree: %s", ref)
	}

	return firstLine(output), nil
}

func RefList(a, b string) ([]string, error) {
	ref := fmt.Sprintf("%s...%s", a, b)
	listCmd := exec.Command("git", "rev-list", "--cherry-pick", "--right-only", "--no-merges", ref)
	listCmd.Stderr = nil
	output, err := listCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Can't load rev-list for %s", ref)
	}

	return outputLines(output), nil
}

func NewRange(a, b string) (*Range, error) {
	parseCmd := exec.Command("git", "rev-parse", "-q", a, b)
	parseCmd.Stderr = nil
	output, err := parseCmd.Output()
	if err != nil {
		return nil, err
	}

	lines := outputLines(output)
	if len(lines) != 2 {
		return nil, fmt.Errorf("Can't parse range %s..%s", a, b)
	}
	return &Range{lines[0], lines[1]}, nil
}

type Range struct {
	A string
	B string
}

func (r *Range) IsIdentical() bool {
	return strings.EqualFold(r.A, r.B)
}

func (r *Range) IsAncestor() bool {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", r.A, r.B)
	return cmd.Run() != nil
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
	cmd.Stderr = nil

	output, err := cmd.Output()
	return strings.TrimSpace(string(output)), err
}

func Log(sha1, sha2 string) (string, error) {
	shaRange := fmt.Sprintf("%s...%s", sha1, sha2)
	cmd := exec.Command(
		"-c", "log.showSignature=false", "log", "--no-color",
		"--format=%h (%aN, %ar)%n%w(78,3,3)%s%n%+b",
		"--cherry", shaRange)
	outputs, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("Can't load git log %s..%s", sha1, sha2)
	}

	return string(outputs), nil
}

func Remotes() ([]string, error) {
	remoteCmd := exec.Command("git", "remote", "-v")
	remoteCmd.Stderr = nil
	output, err := remoteCmd.Output()
	return outputLines(output), err
}

func Config(name string) (string, error) {
	return gitGetConfig(name)
}

func ConfigAll(name string) ([]string, error) {
	mode := "--get-all"
	if strings.Contains(name, "*") {
		mode = "--get-regexp"
	}

	configCmd := exec.Command("git", gitConfigCommand([]string{mode, name})...)
	output, err := configCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Unknown config %s", name)
	}
	return outputLines(output), nil
}

func GlobalConfig(name string) (string, error) {
	return gitGetConfig("--global", name)
}

func SetGlobalConfig(name, value string) error {
	_, err := gitConfig("--global", name, value)
	return err
}

func gitGetConfig(args ...string) (string, error) {
	configCmd := exec.Command("git", gitConfigCommand(args)...)
	output, err := configCmd.Output()
	if err != nil {
		return "", fmt.Errorf("Unknown config %s", args[len(args)-1])
	}

	return firstLine(output), nil
}

func gitConfig(args ...string) ([]string, error) {
	configCmd := exec.Command("git", gitConfigCommand(args)...)
	output, err := configCmd.Output()
	return outputLines(output), err
}

func gitConfigCommand(args []string) []string {
	cmd := []string{"config"}
	return append(cmd, args...)
}

func Alias(name string) (string, error) {
	return Config(fmt.Sprintf("alias.%s", name))
}

func Run(args ...string) error {
	cmd := exec.Command("git", args...)
	return cmd.Run()
}

func LocalBranches() ([]string, error) {
	branchesCmd := exec.Command("git", "branch", "--list")
	output, err := branchesCmd.Output()
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
	if lines == "" {
		return []string{}
	}
	return strings.Split(lines, "\n")

}

func firstLine(output []byte) string {
	if i := bytes.IndexAny(output, "\n"); i >= 0 {
		return string(output)[0:i]
	}
	return string(output)
}
