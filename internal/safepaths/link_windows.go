package safepaths

import (
	"errors"
	"os"
	"syscall"
)

// os.Root intentionally refuses every final reparse point. Windows does not
// expose a supported handle-relative open that both follows non-link reparse
// data and atomically refuses a race to a name-surrogate redirect, so fail
// closed rather than re-resolving the pinned parent by path.
var errUnsupportedNonLinkReparse = errors.New("non-link reparse points cannot be safely opened beneath a rooted directory on Windows")

func isPathRedirect(owner *Root, root *os.Root, name string, info os.FileInfo) (bool, error) {
	return classifyWindowsPathRedirect(info, func() error {
		_, err := owner.readlink(root, name)
		return err
	})
}

func classifyWindowsPathRedirect(info os.FileInfo, readlink func() error) (bool, error) {
	if info.Mode()&os.ModeSymlink != 0 {
		return true, nil
	}
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok || data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		return false, nil
	}
	if err := readlink(); err != nil {
		if os.IsNotExist(err) {
			return false, errUnsupportedNonLinkReparse
		}
		return false, err
	}
	return true, nil
}
