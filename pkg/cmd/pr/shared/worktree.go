package shared

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/cli/v2/git"
)

// WorktreeTarget describes whether a path is a new worktree location or the
// root of an existing linked worktree that can be reused.
type WorktreeTarget struct {
	Path  string
	Reuse bool
}

// ResolveWorktreeTarget validates path and determines whether it is the root
// of an existing linked worktree for the current repository.
func ResolveWorktreeTarget(client *git.Client, path string) (WorktreeTarget, error) {
	target := WorktreeTarget{Path: path}
	if err := ensureWorktreePathSafe(path); err != nil {
		return target, err
	}

	reuse, err := resolveWorktreeTarget(client, path)
	if err != nil {
		return target, err
	}
	if !reuse {
		if err := ensureWorktreePathEmpty(path); err != nil {
			return target, err
		}
	}
	target.Reuse = reuse
	return target, nil
}

// WorktreeCheckoutCommands returns the commands needed to place branch in
// target. startPoint must be a remote branch suitable for --track.
func WorktreeCheckoutCommands(client *git.Client, target WorktreeTarget, branch, startPoint string) ([][]string, bool) {
	branchExists := client.HasLocalBranch(context.Background(), branch)

	if target.Reuse {
		if branchExists {
			return [][]string{{"-C", target.Path, "checkout", branch}}, true
		}
		return [][]string{{"-C", target.Path, "checkout", "-b", branch, "--track", startPoint}}, false
	}

	if branchExists {
		return [][]string{{"worktree", "add", "--", target.Path, branch}}, true
	}
	return [][]string{{"worktree", "add", "--track", "-b", branch, "--", target.Path, startPoint}}, false
}

// resolveWorktreeTarget asks git where path lives, letting git resolve symlinks,
// "..", case, and trailing slashes for us instead of comparing paths ourselves.
// It returns whether an existing linked worktree there should be reused, and
// errors when the path cannot host a new worktree: a path inside a different
// repository, a subdirectory of another worktree, or the worktree we are already
// running in. Detection is best-effort: if git cannot resolve the current or
// target worktree (e.g. the path does not exist yet), reuse is false so git
// worktree add handles the path.
func resolveWorktreeTarget(client *git.Client, path string) (reuseWorktree bool, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}

	// git emits one line per flag, so we expect exactly two lines here.
	current, ok := revParseFacts(client, "", "--show-toplevel", "--git-common-dir")
	if !ok || len(current) != 2 {
		return false, nil
	}
	currentToplevel, currentCommonDir := current[0], current[1]

	// A non-existent or non-git target fails here: it is a fresh path for a new worktree.
	target, ok := revParseFacts(client, abs, "--show-toplevel", "--show-prefix", "--git-common-dir")
	if !ok || len(target) != 3 {
		return false, nil
	}
	targetToplevel, targetPrefix, targetCommonDir := target[0], target[1], target[2]

	switch {
	case targetCommonDir != currentCommonDir:
		return false, fmt.Errorf("--worktree path is inside a different repository")
	case targetToplevel == currentToplevel:
		return false, fmt.Errorf("--worktree path points to the repository you're already in; omit --worktree to check out here")
	case targetPrefix != "":
		return false, fmt.Errorf("--worktree path is inside an existing worktree")
	}
	return true, nil
}

func revParseFacts(client *git.Client, dir string, flags ...string) (fields []string, ok bool) {
	args := append([]string{"rev-parse", "--path-format=absolute"}, flags...)
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd, err := client.Command(context.Background(), args...)
	if err != nil {
		return nil, false
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n"), true
}

func ensureWorktreePathSafe(path string) error {
	fi, err := os.Lstat(path)
	switch {
	case os.IsNotExist(err):
		return nil
	case err != nil:
		return err
	case fi.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("--worktree path must not be a symlink: %s", path)
	case !fi.IsDir():
		return fmt.Errorf("--worktree path must be a directory: %s", path)
	}
	return nil
}

func ensureWorktreePathEmpty(path string) error {
	dir, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()

	_, err = dir.Readdirnames(1)
	switch {
	case errors.Is(err, io.EOF):
		return nil
	case err != nil:
		return err
	default:
		return fmt.Errorf("--worktree path must be empty: %s", path)
	}
}
