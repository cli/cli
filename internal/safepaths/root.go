package safepaths

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Root restricts filesystem operations to a directory tree and refuses to
// traverse symbolic links below that directory.
//
// The caller owns each returned Root and must call Close when it is
// no longer needed.
type Root struct {
	root     *os.Root
	openRoot func(*os.Root, string) (*os.Root, error)
	openFile func(*os.Root, string, int, os.FileMode) (*os.File, error)
	lstat    func(*os.Root, string) (os.FileInfo, error)
	readlink func(*os.Root, string) (string, error)
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
		readlink: func(root *os.Root, name string) (string, error) {
			return root.Readlink(name)
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

// OpenFile creates a file without traversing symbolic links below its rooted
// parent. Creation is always exclusive, including when replace is false.
// filePerm and dirPerm specify creation modes before applying the umask.
func OpenFile(path string, filePerm, dirPerm os.FileMode, replace bool) (*os.File, error) {
	if hasTrailingPathSeparator(path) {
		return nil, trailingPathSeparatorError(path)
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, dirPerm); err != nil {
		return nil, err
	}
	root, err := OpenRoot(parent)
	if err != nil {
		return nil, err
	}
	file, createErr := root.Create(filepath.Base(path), filePerm, dirPerm, replace)
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
// Creation permissions follow [OpenFile].
func WriteFile(path string, data []byte, filePerm, dirPerm os.FileMode, replace bool) (err error) {
	file, err := OpenFile(path, filePerm, dirPerm, replace)
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

// Sub creates and opens a rooted subdirectory.
func (r *Root) Sub(name string, perm os.FileMode) (*Root, error) {
	local, err := r.localPath(name)
	if err != nil {
		return nil, err
	}
	root, err := r.openDirectories(local, perm)
	if err != nil {
		return nil, err
	}

	return &Root{
		root:     root,
		openRoot: r.openRoot,
		openFile: r.openFile,
		lstat:    r.lstat,
		readlink: r.readlink,
		stat:     r.stat,
	}, nil
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
	root, err := r.openDirectories(local, perm)
	if err != nil {
		return err
	}
	return root.Close()
}

// Validate checks name using the same lexical rules as [ValidateChild].
func (r *Root) Validate(name string) error {
	_, err := r.localPath(name)
	return err
}

// Create creates a new file beneath the root. Creation is exclusive unless an
// existing regular file is safely opened and truncated for replacement.
// filePerm and dirPerm specify creation modes before applying the umask.
// Existing regular files and directories retain their permissions.
func (r *Root) Create(name string, filePerm, dirPerm os.FileMode, replace bool) (*os.File, error) {
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
	parent, err := r.openDirectories(filepath.Dir(local), dirPerm)
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
				file, err := r.openVerifiedEntry(parent, base, local)
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

	return parent.OpenFile(base, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
}

// WriteFile writes a complete file beneath the root.
// Creation permissions follow [Root.Create].
func (r *Root) WriteFile(name string, data []byte, filePerm, dirPerm os.FileMode, replace bool) (err error) {
	file, err := r.Create(name, filePerm, dirPerm, replace)
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
// Creation permissions follow [Root.Create].
func (r *Root) CopyFile(name string, src io.Reader, filePerm, dirPerm os.FileMode, replace bool) (err error) {
	file, err := r.Create(name, filePerm, dirPerm, replace)
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

func (r *Root) localPath(name string) (string, error) {
	return validateChildPath(name)
}

// ValidateChild reports whether the cleaned name is a lexically contained path,
// using the host operating system's path rules. Joining it to a root stays
// lexically within that root. Nested paths and names that clean to "." are allowed.
//
// This check does not consult the filesystem or the current working directory,
// and does not account for symbolic links.
func ValidateChild(name string) error {
	_, err := validateChildPath(name)
	return err
}

func validateChildPath(name string) (string, error) {
	local := filepath.Clean(name)
	if !filepath.IsLocal(local) {
		return "", PathTraversalError{Elems: []string{name}}
	}
	return local, nil
}

func (r *Root) openDirectories(name string, perm os.FileMode) (*os.Root, error) {
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
		if os.IsNotExist(err) {
			if err := parent.Mkdir(component, perm); err != nil && !os.IsExist(err) {
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

func (r *Root) openVerifiedEntry(parent *os.Root, name, displayPath string) (*os.File, error) {
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
	if before.Mode()&os.ModeNamedPipe != 0 {
		return nil, namedPipeWriteError(displayPath)
	}

	openFlag := os.O_WRONLY | nonBlockingOpenFlag()
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
	if openFlag != os.O_WRONLY {
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
