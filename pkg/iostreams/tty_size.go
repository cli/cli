//+build !windows

package iostreams

import (
	"os"

	"golang.org/x/term"
)

// ttySize measures the size of the controlling terminal for the current process
func ttySize() (int, int, error) {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return -1, -1, err
	}
	defer f.Close()
	return term.GetSize(int(f.Fd()))
}
