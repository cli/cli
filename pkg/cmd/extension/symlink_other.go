//go:build !windows
// +build !windows

package extension

import "os"

func makeSymlink(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}
