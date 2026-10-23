package cliutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Inside reports whether a cleaned path remains within a directory.
func Inside(directory, candidate string) bool {
	relative, err := filepath.Rel(directory, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative)
}

// ResolvePath resolves existing ancestors without creating the requested path.
func ResolvePath(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	ancestor := absolute
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			resolved, err := filepath.EvalSymlinks(ancestor)
			if err != nil {
				return "", err
			}
			relative, err := filepath.Rel(ancestor, absolute)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolved, relative), nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if filepath.Dir(ancestor) == ancestor {
			return "", fmt.Errorf("path has no readable existing ancestor")
		}
		ancestor = filepath.Dir(ancestor)
	}
}

// RepositoryRoot finds only an enclosing repository, including a worktree marker.
func RepositoryRoot(start string) (string, error) {
	path, err := ResolvePath(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
			return path, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if filepath.Dir(path) == path {
			return "", nil
		}
		path = filepath.Dir(path)
	}
}

// PrivateDirectory creates or verifies a real directory private to this process owner.
func PrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("private directory must not be a symbolic link")
	}
	return CheckPrivate(info)
}

// ConfinedDirectory prepares only nonsymlinked descendants of an owned workspace.
func ConfinedDirectory(workspace, relative string) (string, error) {
	destination := filepath.Join(workspace, relative)
	if filepath.IsAbs(relative) || !Inside(workspace, destination) {
		return "", fmt.Errorf("directory leaves the private workspace")
	}
	current := workspace
	for component := range strings.SplitSeq(filepath.Clean(relative), string(os.PathSeparator)) {
		if component == "." {
			continue
		}
		current = filepath.Join(current, component)
		if err := PrivateDirectory(current); err != nil {
			return "", err
		}
	}
	return destination, nil
}
