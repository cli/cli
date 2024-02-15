package jsonmerge

import (
	"io"
)

type nopMerger struct{}

func NewNopMerger() Merger {
	return &nopMerger{}
}

func (m *nopMerger) NewPage(r io.Reader, _ bool) io.ReadCloser {
	return io.NopCloser(r)
}

func (m *nopMerger) Close() error {
	return nil
}
