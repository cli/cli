package cmdutil

import (
	"io"
	"io/ioutil"
)

func ReadFile(filename string, stdin io.ReadCloser) ([]byte, error) {
	if filename == "-" {
		b, err := ioutil.ReadAll(stdin)
		_ = stdin.Close()
		return b, err
	}

	return ioutil.ReadFile(filename)
}
