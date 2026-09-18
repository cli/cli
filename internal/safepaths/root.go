package safepaths

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Root restricts filesystem operations to a directory tree and refuses to
// traverse symbolic links below that directory.
type Root struct {
	path     Absolute
	root     *os.Root
	openRoot func(*os.Root, string) (*os.Root, error)
	openFile func(*os.Root, string, int, os.FileMode) (*os.File, error)
	lstat    func(*os.Root, string) (os.FileInfo, error)
	mkdir    func(*os.Root, string, os.FileMode) error
	random   func([]byte) (int, error)
	readlink func(*os.Root, string) (string, error)
	rename   func(*os.Root, string, string) error
	stat     func(*os.Root, string) (os.FileInfo, error)
}

// OpenRoot opens an existing directory as a rooted filesystem.
func OpenRoot(path string) (*Root, error) {
	absolute, err := ParseAbsolute(path)
	if err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(absolute.String())
	if err != nil {
		return nil, err
	}

	return &Root{
		path: absolute,
		root: root,
		openRoot: func(root *os.Root, name string) (*os.Root, error) {
			return root.OpenRoot(name)
		},
		openFile: func(root *os.Root, name string, flag int, perm os.FileMode) (*os.File, error) {
			return root.OpenFile(name, flag, perm)
		},
		lstat: func(root *os.Root, name string) (os.FileInfo, error) {
			return root.Lstat(name)
		},
		mkdir: func(root *os.Root, name string, perm os.FileMode) error {
			return root.Mkdir(name, perm)
		},
		random: rand.Read,
		readlink: func(root *os.Root, name string) (string, error) {
			return root.Readlink(name)
		},
		rename: func(root *os.Root, oldName, newName string) error {
			return root.Rename(oldName, newName)
		},
		stat: func(root *os.Root, name string) (os.FileInfo, error) {
			return root.Stat(name)
		},
	}, nil
}

// OpenRootDir creates and opens a user-selected directory as a rooted filesystem.
// The selected path may contain symbolic links; child paths beneath the opened
// root may not.
func OpenRootDir(path string, perm os.FileMode) (*Root, error) {
	if path == "" {
		path = "."
	}
	if err := os.MkdirAll(path, perm); err != nil {
		return nil, err
	}
	return OpenRoot(path)
}

// OpenRootWithin opens rootDir and a contained targetDir. It returns both
// handles so callers may keep the trusted parent pinned with the child.
func OpenRootWithin(rootDir, targetDir string, perm os.FileMode) (*Root, *Root, error) {
	if err := os.MkdirAll(rootDir, perm); err != nil {
		return nil, nil, err
	}
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, nil, err
	}
	targetAbs, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, nil, err
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return nil, nil, err
	}
	if rel != "." && !filepath.IsLocal(rel) {
		return nil, nil, fmt.Errorf("target directory %s is outside root %s", targetDir, rootDir)
	}

	root, err := OpenRoot(rootAbs)
	if err != nil {
		return nil, nil, err
	}
	targetRoot, err := root.Sub(rel, perm)
	if err != nil {
		_ = root.Close()
		return nil, nil, err
	}
	return root, targetRoot, nil
}

// OpenFile creates a file without traversing symbolic links below its rooted
// parent. Creation is always exclusive, including when replace is false.
func OpenFile(path string, perm os.FileMode, replace bool) (*os.File, error) {
	if hasTrailingPathSeparator(path) {
		return nil, trailingPathSeparatorError(path)
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	root, err := OpenRoot(parent)
	if err != nil {
		return nil, err
	}
	file, createErr := root.Create(filepath.Base(path), perm, replace)
	closeErr := root.Close()
	if createErr != nil {
		return nil, createErr
	}
	if closeErr != nil {
		_ = file.Close()
		return nil, closeErr
	}
	return file, nil
}

// WriteFile writes a complete file without traversing symbolic links.
func WriteFile(path string, data []byte, perm os.FileMode, replace bool) (err error) {
	file, err := OpenFile(path, perm, replace)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	_, err = file.Write(data)
	return err
}

// Close closes the rooted filesystem.
func (r *Root) Close() error {
	return r.root.Close()
}

// String returns the absolute path to the root directory.
func (r *Root) String() string {
	return r.path.String()
}

// Sub creates and opens a rooted subdirectory.
func (r *Root) Sub(name string, perm os.FileMode) (*Root, error) {
	local, err := r.localPath(name)
	if err != nil {
		return nil, err
	}
	absolute, err := r.path.Join(local)
	if err != nil {
		return nil, err
	}
	root, err := r.openDirectories(local, perm, true)
	if err != nil {
		return nil, err
	}

	return &Root{
		path:     absolute,
		root:     root,
		openRoot: r.openRoot,
		openFile: r.openFile,
		lstat:    r.lstat,
		mkdir:    r.mkdir,
		random:   r.random,
		readlink: r.readlink,
		rename:   r.rename,
		stat:     r.stat,
	}, nil
}

// Lstat returns information about a path without following a final symbolic
// link. Symbolic links in parent components are rejected.
func (r *Root) Lstat(name string) (os.FileInfo, error) {
	local, err := r.localPath(name)
	if err != nil {
		return nil, err
	}
	parent, err := r.openDirectories(filepath.Dir(local), 0, false)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	return parent.Lstat(filepath.Base(local))
}

// MkdirAll creates a directory path without traversing symbolic links.
func (r *Root) MkdirAll(name string, perm os.FileMode) error {
	local, err := r.localPath(name)
	if err != nil {
		return err
	}
	if local == "." {
		return nil
	}
	root, err := r.openDirectories(local, perm, true)
	if err != nil {
		return err
	}
	return root.Close()
}

// Validate reports whether name is lexically contained beneath the root.
func (r *Root) Validate(name string) error {
	_, err := r.localPath(name)
	return err
}

// Create creates a new file beneath the root. Creation is exclusive unless an
// existing regular file is safely opened and truncated for replacement.
func (r *Root) Create(name string, perm os.FileMode, replace bool) (*os.File, error) {
	if hasTrailingPathSeparator(name) {
		return nil, trailingPathSeparatorError(name)
	}
	local, err := r.localPath(name)
	if err != nil {
		return nil, err
	}
	if local == "." {
		return nil, fmt.Errorf("cannot create root directory as a file")
	}
	parent, err := r.openDirectories(filepath.Dir(local), 0o755, true)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	base := filepath.Base(local)

	if replace {
		info, err := r.lstat(parent, base)
		if err == nil {
			redirect, err := isPathRedirect(r, parent, base, info)
			if err != nil {
				return nil, err
			}
			if redirect {
				targetIsDir, err := r.redirectTargetIsDir(parent, base)
				if err != nil {
					return nil, err
				}
				if targetIsDir {
					return nil, directoryWriteError(name)
				}
				if err := parent.Remove(base); err != nil {
					return nil, err
				}
			} else {
				if info.IsDir() {
					return nil, directoryWriteError(name)
				}
				if info.Mode()&os.ModeNamedPipe != 0 {
					return nil, namedPipeWriteError(name)
				}
				file, err := r.openVerifiedEntry(parent, base, local, os.O_WRONLY, false)
				if err != nil {
					return nil, err
				}
				if info.Mode().IsRegular() || info.Mode()&os.ModeIrregular != 0 {
					if err := file.Truncate(0); err != nil {
						_ = file.Close()
						return nil, err
					}
					if _, err := file.Seek(0, io.SeekStart); err != nil {
						_ = file.Close()
						return nil, err
					}
				}
				return file, nil
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return parent.OpenFile(base, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
}

// WriteFile writes a complete file beneath the root.
func (r *Root) WriteFile(name string, data []byte, perm os.FileMode, replace bool) (err error) {
	file, err := r.Create(name, perm, replace)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	_, err = file.Write(data)
	return err
}

// CopyFile copies content into a file beneath the root.
func (r *Root) CopyFile(name string, src io.Reader, perm os.FileMode, replace bool) (err error) {
	file, err := r.Create(name, perm, replace)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	_, err = io.Copy(file, src)
	return err
}

// TempDir creates and opens a uniquely named directory directly beneath the root.
func (r *Root) TempDir(prefix string) (string, *Root, error) {
	if prefix == "" || filepath.Base(prefix) != prefix || !filepath.IsLocal(prefix) {
		return "", nil, fmt.Errorf("invalid temporary directory prefix %q", prefix)
	}

	for range 100 {
		var random [8]byte
		if _, err := r.random(random[:]); err != nil {
			return "", nil, err
		}
		name := prefix + hex.EncodeToString(random[:])
		if err := r.mkdir(r.root, name, 0o700); err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", nil, err
		}

		root, err := r.Sub(name, 0o700)
		if err != nil {
			_ = r.root.RemoveAll(name)
			return "", nil, err
		}
		return name, root, nil
	}
	return "", nil, fmt.Errorf("could not create a unique temporary directory")
}

// ReadDir reads a directory beneath the root without traversing symbolic links.
func (r *Root) ReadDir(name string) ([]os.DirEntry, error) {
	local, err := r.localPath(name)
	if err != nil {
		return nil, err
	}
	root, err := r.openDirectories(local, 0, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return dir.ReadDir(-1)
}

// Open opens a regular file beneath the root without traversing symbolic links.
func (r *Root) Open(name string) (*os.File, error) {
	local, err := r.localPath(name)
	if err != nil {
		return nil, err
	}
	if local == "." {
		return nil, fmt.Errorf("cannot open root directory as a file")
	}
	parent, err := r.openDirectories(filepath.Dir(local), 0, false)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	base := filepath.Base(local)

	return r.openVerifiedEntry(parent, base, local, os.O_RDONLY, true)
}

// Rename renames an entry beneath one pinned, verified parent directory.
func (r *Root) Rename(oldName, newName string) error {
	oldLocal, err := r.localPath(oldName)
	if err != nil {
		return err
	}
	newLocal, err := r.localPath(newName)
	if err != nil {
		return err
	}
	oldParent := filepath.Dir(oldLocal)
	if oldParent != filepath.Dir(newLocal) {
		return fmt.Errorf("rooted rename requires a common parent directory")
	}
	parent, err := r.openDirectories(oldParent, 0, false)
	if err != nil {
		return err
	}
	defer parent.Close()
	return r.rename(parent, filepath.Base(oldLocal), filepath.Base(newLocal))
}

// RemoveAll removes a path beneath the root without following a final symbolic link.
func (r *Root) RemoveAll(name string) error {
	local, err := r.localPath(name)
	if err != nil {
		return err
	}
	if local == "." {
		return fmt.Errorf("cannot remove root directory")
	}
	parent, err := r.openDirectories(filepath.Dir(local), 0, false)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer parent.Close()
	return parent.RemoveAll(filepath.Base(local))
}

func (r *Root) localPath(name string) (string, error) {
	return validateChildPath(name)
}

// ValidateChild reports whether name is a lexically contained child path.
func ValidateChild(name string) error {
	_, err := validateChildPath(name)
	return err
}

func validateChildPath(name string) (string, error) {
	local := filepath.Clean(filepath.FromSlash(name))
	if !filepath.IsLocal(local) {
		return "", PathTraversalError{Elems: []string{name}}
	}
	return local, nil
}

func (r *Root) openDirectories(name string, perm os.FileMode, create bool) (*os.Root, error) {
	var current string
	parent := r.root
	parentOwned := false
	closeParent := func() {
		if parentOwned {
			_ = parent.Close()
		}
	}

	if name == "." {
		root, err := r.openVerifiedDirectory(parent, ".", ".")
		if err != nil {
			return nil, err
		}
		return root, nil
	}

	for component := range strings.SplitSeq(name, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)

		info, err := parent.Lstat(component)
		if os.IsNotExist(err) && create {
			if err := r.mkdir(parent, component, perm); err != nil && !os.IsExist(err) {
				closeParent()
				return nil, err
			}
			info, err = parent.Lstat(component)
		}
		if err != nil {
			closeParent()
			return nil, err
		}
		redirect, redirectErr := isPathRedirect(r, parent, component, info)
		if redirectErr != nil {
			closeParent()
			return nil, redirectErr
		}
		if redirect {
			closeParent()
			return nil, SymlinkTraversalError{Path: current}
		}
		if !info.IsDir() {
			closeParent()
			return nil, &os.PathError{Op: "open", Path: current, Err: fmt.Errorf("not a directory")}
		}

		next, err := r.openVerifiedDirectory(parent, component, current)
		if err != nil {
			closeParent()
			return nil, err
		}
		if parentOwned {
			if err := parent.Close(); err != nil {
				_ = next.Close()
				return nil, err
			}
		}
		parent = next
		parentOwned = true
	}
	return parent, nil
}

func (r *Root) openVerifiedDirectory(parent *os.Root, name, displayPath string) (*os.Root, error) {
	before, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	redirect, err := isPathRedirect(r, parent, name, before)
	if err != nil {
		return nil, err
	}
	if redirect {
		return nil, SymlinkTraversalError{Path: displayPath}
	}

	root, err := r.openRoot(parent, name)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	after, err := parent.Lstat(name)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	redirect, err = isPathRedirect(r, parent, name, after)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	if redirect {
		_ = root.Close()
		return nil, SymlinkTraversalError{Path: displayPath}
	}
	if !os.SameFile(before, opened) || !os.SameFile(before, after) {
		_ = root.Close()
		return nil, fmt.Errorf("directory %q changed while opening", displayPath)
	}
	return root, nil
}

func (r *Root) openVerifiedEntry(parent *os.Root, name, displayPath string, flag int, requireRegular bool) (*os.File, error) {
	before, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	redirect, err := isPathRedirect(r, parent, name, before)
	if err != nil {
		return nil, err
	}
	if redirect {
		return nil, SymlinkTraversalError{Path: displayPath}
	}
	if requireRegular && !before.Mode().IsRegular() {
		return nil, &os.PathError{Op: "open", Path: displayPath, Err: fmt.Errorf("not a regular file")}
	}
	if !requireRegular && before.Mode()&os.ModeNamedPipe != 0 {
		return nil, namedPipeWriteError(displayPath)
	}

	openFlag := flag
	if !requireRegular && flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		openFlag |= nonBlockingOpenFlag()
	}
	file, err := r.openFile(parent, name, openFlag, 0)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	after, err := parent.Lstat(name)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	redirect, err = isPathRedirect(r, parent, name, after)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if redirect {
		_ = file.Close()
		return nil, SymlinkTraversalError{Path: displayPath}
	}
	if !os.SameFile(before, opened) || !os.SameFile(before, after) {
		_ = file.Close()
		return nil, fmt.Errorf("file %q changed while opening", displayPath)
	}
	if openFlag != flag {
		if err := clearNonblocking(file); err != nil {
			_ = file.Close()
			return nil, err
		}
	}
	return file, nil
}

func (r *Root) redirectTargetIsDir(parent *os.Root, name string) (bool, error) {
	info, err := r.stat(parent, name)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("could not safely classify redirect target %q: %w", name, err)
	}
	return info.IsDir(), nil
}

func directoryWriteError(path string) error {
	return &os.PathError{Op: "open", Path: path, Err: syscall.EISDIR}
}

func namedPipeWriteError(path string) error {
	return &os.PathError{Op: "open", Path: path, Err: fmt.Errorf("named pipes cannot be overwritten safely")}
}

// SymlinkTraversalError reports a symbolic link in a rooted path.
type SymlinkTraversalError struct {
	Path string
}

func (e SymlinkTraversalError) Error() string {
	return fmt.Sprintf("refusing to traverse symbolic link %q", e.Path)
}

func hasTrailingPathSeparator(path string) bool {
	return len(path) > 0 && os.IsPathSeparator(path[len(path)-1])
}

func trailingPathSeparatorError(path string) error {
	return &os.PathError{Op: "open", Path: path, Err: fmt.Errorf("file path ends in a separator")}
}
