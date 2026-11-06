package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Git fixtures contain both the remote repositories and the local checkouts.
// A run snapshots the whole fixture once, then copies it for each gh build.
const gitFixture = "git/fixture"

type gitOutcome struct {
	Directory string `json:"directory"`
	Version   string `json:"version"`
}

var gitNamePart = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
var gitHostName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)

func parseGitRepo(name string) (host, repo string, err error) {
	parts := strings.Split(name, "/")
	host = dotcomHost
	if len(parts) == 3 {
		host, parts = parts[0], parts[1:]
	}
	if !gitHostName.MatchString(host) || len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repository %q; use [HOST/]OWNER/REPO", name)
	}
	parts[1] = strings.TrimSuffix(parts[1], ".git")
	for _, p := range parts {
		if !gitNamePart.MatchString(p) || strings.HasSuffix(p, ".") {
			return "", "", fmt.Errorf("invalid repository %q; use [HOST/]OWNER/REPO", name)
		}
	}
	return strings.ToLower(host), strings.Join(parts, "/"), nil
}

func gitRepoPath(world, host, repo string) string {
	if host == mockHost || host == "ssh.github.com" {
		host = dotcomHost
	}
	return filepath.Join(world, "repos", host, filepath.FromSlash(repo)+".git")
}

func (s *state) gitFixtureDir() string {
	return filepath.Join(s.dir, filepath.FromSlash(gitFixture))
}

func (s *state) ensureGitFixture() (string, error) {
	dir := s.gitFixtureDir()
	for _, name := range []string{"work", "repos"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			return "", err
		}
	}
	return filepath.EvalSymlinks(dir)
}

func (s *state) snapshotGit(runID, in string) (string, error) {
	_, err := os.Stat(s.gitFixtureDir())
	if errors.Is(err, fs.ErrNotExist) {
		if in != "" {
			return "", fmt.Errorf("no Git fixture yet; prepare it with ghmock repo, git or write")
		}
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if _, err := gitWorkDir(s.gitFixtureDir(), in); err != nil {
		return "", err
	}
	relative := filepath.Join("git", "runs", runID, "before")
	if err := copyGitTree(s.gitFixtureDir(), filepath.Join(s.dir, relative)); err != nil {
		return "", fmt.Errorf("snapshotting Git fixture: %w", err)
	}
	return filepath.ToSlash(relative), nil
}

func gitWorkDir(world, in string) (string, error) {
	if in == "" {
		in = "."
	}
	if !filepath.IsLocal(in) {
		return "", fmt.Errorf("--in must be a relative path inside the Git workspace")
	}
	root, err := filepath.EvalSymlinks(filepath.Join(world, "work"))
	if err != nil {
		return "", err
	}
	dir, err := filepath.EvalSymlinks(filepath.Join(root, in))
	if err != nil {
		return "", fmt.Errorf("Git workspace %q: %w", in, err)
	}
	if !pathWithin(root, dir) {
		return "", fmt.Errorf("Git workspace %q points outside the fixture", in)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("Git workspace %q is not a directory", in)
	}
	return dir, nil
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(rel)
}

// copyGitTree preserves the index, untracked files and worktree metadata, which
// a clone would lose. It copies file contents, never hardlinks. Only Git's path
// files and internal absolute symlinks are relocated; user file contents stay
// unchanged.
func copyGitTree(source, destination string) error {
	source, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}
	if pathWithin(source, destination) {
		return fmt.Errorf("cannot copy Git state into itself")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(destination, 0o755); err != nil {
		return err
	}
	destination, err = filepath.EvalSymlinks(destination)
	if err != nil {
		return err
	}
	if pathWithin(source, destination) {
		return fmt.Errorf("cannot copy Git state into itself")
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil || rel == "." {
			return err
		}
		target := filepath.Join(destination, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.Mkdir(target, info.Mode().Perm()|0o700)
		case entry.Type()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if filepath.IsAbs(link) && pathWithin(source, link) {
				relative, err := filepath.Rel(source, link)
				if err != nil {
					return err
				}
				link = filepath.Join(destination, relative)
			}
			return os.Symlink(link, target)
		case !info.Mode().IsRegular():
			return fmt.Errorf("cannot copy Git fixture file %s: not a regular file or symlink", rel)
		}
		if isGitPathFile(filepath.ToSlash(rel)) {
			return copyGitPathFile(path, target, source, destination, info.Mode().Perm())
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		return errors.Join(copyErr, closeErr)
	})
}

func isGitPathFile(path string) bool {
	if strings.HasSuffix(path, "/objects/info/alternates") {
		return true
	}
	if filepath.Base(path) == ".git" {
		return true
	}
	return strings.Contains(path, ".git/worktrees/") &&
		(filepath.Base(path) == "gitdir" || filepath.Base(path) == "commondir")
}

func copyGitPathFile(sourceFile, destinationFile, source, destination string, mode fs.FileMode) error {
	if strings.HasSuffix(filepath.ToSlash(sourceFile), "/objects/info/alternates") {
		data, err := relocateGitAlternates(sourceFile, source, destination)
		if err != nil {
			return err
		}
		return os.WriteFile(destinationFile, data, mode)
	}
	data, err := os.ReadFile(sourceFile)
	if err != nil {
		return err
	}
	line := strings.TrimSuffix(string(data), "\n")
	prefix := ""
	if after, ok := strings.CutPrefix(line, "gitdir: "); ok {
		prefix, line = "gitdir: ", after
	}
	if filepath.IsAbs(line) {
		// Git on macOS may record /private/var while TempDir returns /var.
		resolved, err := filepath.EvalSymlinks(line)
		if err != nil {
			return fmt.Errorf("relocating Git path %s: %w", sourceFile, err)
		}
		if !pathWithin(source, resolved) {
			return fmt.Errorf("Git path %s points outside the fixture: %s", sourceFile, line)
		}
		rel, err := filepath.Rel(source, resolved)
		if err != nil {
			return err
		}
		data = []byte(prefix + filepath.Join(destination, rel) + "\n")
	}
	return os.WriteFile(destinationFile, data, mode)
}

func relocateGitAlternates(path, source, destination string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	objects := filepath.Dir(filepath.Dir(path))
	relative, err := filepath.Rel(source, objects)
	if err != nil {
		return nil, err
	}
	copiedObjects := filepath.Join(destination, relative)
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		target := line
		if !filepath.IsAbs(target) {
			target = filepath.Join(objects, target)
		}
		target, err = filepath.EvalSymlinks(target)
		if err != nil {
			return nil, fmt.Errorf("relocating Git alternate %s: %w", path, err)
		}
		if !pathWithin(source, target) {
			return nil, fmt.Errorf("Git alternate %s points outside the fixture: %s", path, line)
		}
		relative, err = filepath.Rel(source, target)
		if err != nil {
			return nil, err
		}
		lines[i], err = filepath.Rel(copiedObjects, filepath.Join(destination, relative))
		if err != nil {
			return nil, err
		}
		lines[i] = filepath.ToSlash(lines[i])
	}
	return []byte(strings.Join(lines, "\n")), nil
}
