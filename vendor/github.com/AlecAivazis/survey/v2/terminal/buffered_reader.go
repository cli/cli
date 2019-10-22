package terminal

import (
	"bytes"
	"io"
)

type BufferedReader struct {
	In     io.Reader
	Buffer *bytes.Buffer
}

func (br *BufferedReader) Read(p []byte) (int, error) {
	n, err := br.Buffer.Read(p)
	if err != nil && err != io.EOF {
		return n, err
	} else if err == nil {
		return n, nil
	}

	return br.In.Read(p[n:])
}
