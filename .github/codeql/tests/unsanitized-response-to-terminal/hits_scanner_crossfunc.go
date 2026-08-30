package fixtures

import (
	"bufio"
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// A helper returns the raw body; the caller scans it line by line and prints to
// Out. Must be flagged.
func fetchLog(url string) (io.ReadCloser, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func ScanLogToOut(url string, ios *iostreams.IOStreams) error {
	rc, err := fetchLog(url)
	if err != nil {
		return err
	}
	defer rc.Close()
	scanner := bufio.NewScanner(rc)
	for scanner.Scan() {
		fmt.Fprintf(ios.Out, "%s\n", scanner.Text())
	}
	return nil
}
