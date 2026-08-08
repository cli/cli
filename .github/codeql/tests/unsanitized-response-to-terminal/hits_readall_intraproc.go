package fixtures

import (
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// io.ReadAll of the body, printed to Out in the same function. Must be flagged.
func ReadAllToOut(resp *http.Response, ios *iostreams.IOStreams) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintln(ios.Out, string(body))
	return nil
}
