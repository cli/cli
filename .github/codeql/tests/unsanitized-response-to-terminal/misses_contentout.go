package fixtures

import (
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// Raw body written to the blessed ContentOut writer. Must NOT be flagged.
func ReadAllToContentOut(url string, ios *iostreams.IOStreams) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintln(ios.ContentOut, string(body))
	return nil
}

func CopyToContentOut(url string, ios *iostreams.IOStreams) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(ios.ContentOut, resp.Body)
	return err
}
