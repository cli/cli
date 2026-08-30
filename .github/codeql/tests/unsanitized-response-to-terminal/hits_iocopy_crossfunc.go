package fixtures

import (
	"io"
	"net/http"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// A helper returns the raw body; the caller streams it to Out from a different
// function. Must be flagged.
func fetchBodyForCopy(url string) (io.ReadCloser, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func CopyBodyToOut(url string, ios *iostreams.IOStreams) error {
	r, err := fetchBodyForCopy(url)
	if err != nil {
		return err
	}
	defer r.Close()
	_, err = io.Copy(ios.Out, r)
	return err
}
