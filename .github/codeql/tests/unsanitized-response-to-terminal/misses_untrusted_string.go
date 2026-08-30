package fixtures

import (
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// A body minted as Untrusted and printed through String(), which sanitizes. Must
// NOT be flagged.
func UntrustedStringToOut(resp *http.Response, ios *iostreams.IOStreams) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	u := iostreams.NewUntrustedBytes(body)
	fmt.Fprintln(ios.Out, u.String())
	return nil
}
