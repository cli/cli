// Minimal stub of golang.org/x/text/transform for CodeQL test extraction.
// Only needs NewReader to exist under the expected qualified name.
package transform

import "io"

type Transformer interface {
	Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error)
	Reset()
}

type Reader struct{ r io.Reader }

func (r *Reader) Read(p []byte) (int, error) { return r.r.Read(p) }

func NewReader(r io.Reader, t Transformer) *Reader {
	return &Reader{r: r}
}
