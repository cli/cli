package iostreams

import (
	"os"

	"golang.org/x/term"
)

func ttySize() (int, int, error) {
	f, err := os.Open("CONOUT$")
	if err != nil {
		return -1, -1, err
	}
	defer f.Close()
	return term.GetSize(int(f.Fd()))
}
