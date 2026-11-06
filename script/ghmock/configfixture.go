package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type configOutcome struct {
	Directory string `json:"directory"`
}

func (s *state) snapshotConfig(runID, source string) (string, error) {
	if source == "" {
		return "", nil
	}
	relative := filepath.Join("config", "runs", runID, "before")
	if err := copyConfigTree(source, filepath.Join(s.dir, relative)); err != nil {
		return "", fmt.Errorf("copying configuration fixture: %w", err)
	}
	return filepath.ToSlash(relative), nil
}

// copyConfigTree copies fixture bytes without following contained symlinks.
// Each destination is new and private; no caller configuration is copied implicitly.
func copyConfigTree(source, destination string) (err error) {
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return err
	}
	src, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer src.Close()
	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}
	parent, suffix := filepath.Dir(destination), filepath.Base(destination)
	for {
		resolved, resolveErr := filepath.EvalSymlinks(parent)
		if resolveErr == nil {
			destination = filepath.Join(resolved, suffix)
			break
		}
		if !errors.Is(resolveErr, fs.ErrNotExist) || filepath.Dir(parent) == parent {
			return resolveErr
		}
		suffix = filepath.Join(filepath.Base(parent), suffix)
		parent = filepath.Dir(parent)
	}
	if pathWithin(source, destination) {
		return fmt.Errorf("cannot copy configuration into its source")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(destination))
		}
	}()
	dst, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer dst.Close()
	return fs.WalkDir(src.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || name == "." {
			return walkErr
		}
		if entry.IsDir() {
			return dst.Mkdir(filepath.FromSlash(name), 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("configuration fixture %s: not a regular file or directory", name)
		}
		input, err := src.Open(filepath.FromSlash(name))
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := dst.OpenFile(filepath.FromSlash(name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		return errors.Join(copyErr, output.Close())
	})
}
