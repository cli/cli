package jsonmerge

import (
	"bytes"
	"encoding/json"
	"io"

	"dario.cat/mergo"
)

type objectMerger struct {
	io.Writer
	dst map[string]interface{}
}

// NewObjectMerger creates a Merger for JSON objects.
func NewObjectMerger(w io.Writer) Merger {
	return &objectMerger{
		Writer: w,
		dst:    make(map[string]interface{}),
	}
}

func (merger *objectMerger) NewPage(r io.Reader, isLastPage bool) io.ReadCloser {
	return &objectMergerPage{
		merger: merger,
		Reader: r,
	}
}

func (merger *objectMerger) Close() error {
	// Marshal to JSON and write to output.
	buf, err := json.Marshal(merger.dst)
	if err != nil {
		return err
	}

	_, err = merger.Writer.Write(buf)
	return err
}

type objectMergerPage struct {
	merger *objectMerger

	io.Reader
	buffer bytes.Buffer
}

// Read caches the data in an internal buffer to be merged in Close.
// No data is copied into p so it's not written to stdout.
func (page *objectMergerPage) Read(p []byte) (int, error) {
	_, err := io.CopyN(&page.buffer, page.Reader, int64(len(p)))
	return 0, err
}

// Close converts the internal buffer to a JSON object and merges it with the final JSON object.
func (page *objectMergerPage) Close() error {
	var src map[string]interface{}

	err := json.Unmarshal(page.buffer.Bytes(), &src)
	if err != nil {
		return err
	}

	return mergo.Merge(&page.merger.dst, src, mergo.WithAppendSlice, mergo.WithOverride)
}
