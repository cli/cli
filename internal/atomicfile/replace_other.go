//go:build !windows

package atomicfile

import "os"

func replace(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
