//go:build !windows

package safepaths

import "os"

func isPathRedirect(_ *Root, _ *os.Root, _ string, info os.FileInfo) (bool, error) {
	return info.Mode()&os.ModeSymlink != 0, nil
}
