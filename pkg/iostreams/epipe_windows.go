package iostreams

import (
	"errors"
	"syscall"
)

func isEpipeError(err error) bool {
	// 232 is Windows error code ERROR_NO_DATA, "The pipe is being closed".
	return errors.Is(err, syscall.Errno(232))
}
