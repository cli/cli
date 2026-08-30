package fixtures

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// The body is written to a disk cache, then reopened and printed. Static taint
// cannot bridge the filesystem, so the query is silent here by necessity. The
// runtime ContentOut writer is what covers this case. Documented as a known
// limitation; must NOT be flagged.
func cacheBody(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func PrintCachedFile(path string, ios *iostreams.IOStreams) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fmt.Fprintf(ios.Out, "%s\n", scanner.Text())
	}
	return nil
}
