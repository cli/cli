//go:build !unix

package safepaths

import "os"

func nonBlockingOpenFlag() int {
	return 0
}

func clearNonblocking(*os.File) error {
	return nil
}
