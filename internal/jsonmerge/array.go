package jsonmerge

import "io"

type arrayMerger struct {
	isFirstPage bool
}

// NewArrayMerger creates a Merger for JSON arrays.
func NewArrayMerger() Merger {
	return &arrayMerger{
		isFirstPage: true,
	}
}

func (merger *arrayMerger) NewPage(r io.Reader, isLastPage bool) io.ReadCloser {
	return &arrayMergerPage{
		merger:     merger,
		Reader:     r,
		isLastPage: isLastPage,
	}
}

func (m *arrayMerger) Close() error {
	// arrayMerger merges when reading, so any output was already written
	// and there's nothing to do on Close.
	return nil
}

type arrayMergerPage struct {
	merger *arrayMerger

	io.Reader
	isLastPage bool

	isSubsequentRead bool
	cachedByte       byte
}

func (page *arrayMergerPage) Read(p []byte) (int, error) {
	var n int
	var err error

	if page.cachedByte != 0 && len(p) > 0 {
		p[0] = page.cachedByte
		n, err = page.Reader.Read(p[1:])
		n += 1
		page.cachedByte = 0
	} else {
		n, err = page.Reader.Read(p)
	}

	if !page.isSubsequentRead && !page.merger.isFirstPage && n > 0 && p[0] == '[' {
		if n > 1 && p[1] == ']' {
			// Empty array case.
			p[0] = ' '
		} else {
			// Avoid starting a new array and continue with a comma instead.
			p[0] = ','
		}
	}

	if !page.isLastPage && n > 0 && p[n-1] == ']' {
		// Avoid closing off an array in case we determine we are at EOF.
		page.cachedByte = p[n-1]
		n -= 1
	}

	page.isSubsequentRead = true
	return n, err
}

func (page *arrayMergerPage) Close() error {
	page.merger.isFirstPage = false
	return nil
}
