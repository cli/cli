package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func writeZip(w io.Writer, root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", root)
	}
	zw := zip.NewWriter(w)
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("cannot ZIP %s: not a regular file", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		header.Method = zip.Deflate
		dst, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(dst, file)
		return errors.Join(copyErr, file.Close())
	})
	return errors.Join(walkErr, zw.Close())
}

func writeExportArchive(path string, write func(string) error) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s already exists; export to a new file", path)
	}
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, file.Close())
		if err != nil {
			err = errors.Join(err, os.Remove(path))
		}
	}()
	dir, err := os.MkdirTemp(filepath.Dir(path), ".ghmock-export-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	if err := write(dir); err != nil {
		return err
	}
	return writeTarGzip(file, dir)
}

func writeTarGzip(w io.Writer, root string) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || path == root {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
			if filepath.IsAbs(link) && pathWithin(root, link) {
				link, err = filepath.Rel(filepath.Dir(path), link)
				if err != nil {
					return err
				}
			}
		}
		header, err := tar.FileInfoHeader(info, filepath.ToSlash(link))
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		var data []byte
		if info.Mode().IsRegular() && strings.HasPrefix(header.Name, "git/") && isGitPathFile(header.Name) {
			data, err = portableGitPath(path, root)
			if err != nil {
				return err
			}
			header.Size = int64(len(data))
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if data != nil {
			_, err := tw.Write(data)
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, file)
		return errors.Join(copyErr, file.Close())
	})
	return errors.Join(walkErr, tw.Close(), gz.Close())
}

func portableGitPath(path, root string) ([]byte, error) {
	if strings.HasSuffix(filepath.ToSlash(path), "/objects/info/alternates") {
		return relocateGitAlternates(path, root, root)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	line := strings.TrimSuffix(string(data), "\n")
	prefix := ""
	if after, ok := strings.CutPrefix(line, "gitdir: "); ok {
		prefix, line = "gitdir: ", after
	}
	if !filepath.IsAbs(line) {
		return data, nil
	}
	if !pathWithin(root, line) {
		return nil, fmt.Errorf("Git path %s points outside the export: %s", path, line)
	}
	rel, err := filepath.Rel(filepath.Dir(path), line)
	if err != nil {
		return nil, err
	}
	return []byte(prefix + filepath.ToSlash(rel) + "\n"), nil
}
