package fixtures

import (
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// A body minted as Untrusted but taken out through Raw and printed. Raw is the
// deliberate opt-out and is not a barrier, so the content reaches the terminal
// raw and must be flagged.
func RawEscapeHatchToOut(resp *http.Response, ios *iostreams.IOStreams) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	u := iostreams.NewUntrustedBytes(body)
	fmt.Fprintln(ios.Out, u.Raw())
	return nil
}
