// +build !windows

package terminal

import (
	"fmt"
)

func EraseLine(out FileWriter, mode EraseLineMode) {
	fmt.Fprintf(out, "\x1b[%dK", mode)
}
