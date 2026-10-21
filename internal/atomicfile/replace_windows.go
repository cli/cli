//go:build windows

package atomicfile

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func replace(oldPath, newPath string) error {
	deadline := time.Now().Add(100 * time.Millisecond)
	for delay := time.Millisecond; ; delay *= 2 {
		err := os.Rename(oldPath, newPath)
		if err == nil {
			return nil
		}
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) &&
			!errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return err
		}
		time.Sleep(min(delay, remaining))
	}
}
