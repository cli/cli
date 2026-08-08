package fixtures

import (
	"bufio"
	"fmt"
	"io"
	"net/http"

	"github.com/cli/go-gh/v2/pkg/asciisanitizer"
	"golang.org/x/text/transform"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// The body is wrapped with the asciisanitizer transform before printing. Must
// NOT be flagged.
func SanitizedScanToOut(url string, ios *iostreams.IOStreams) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	sanitized := transform.NewReader(resp.Body, &asciisanitizer.Sanitizer{})
	scanner := bufio.NewScanner(sanitized)
	for scanner.Scan() {
		fmt.Fprintf(ios.Out, "%s\n", scanner.Text())
	}
	return nil
}

func SanitizedCopyToOut(url string, ios *iostreams.IOStreams) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	sanitized := transform.NewReader(resp.Body, &asciisanitizer.Sanitizer{})
	_, err = io.Copy(ios.Out, sanitized)
	return err
}
