package example

import (
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/pkg/iostreams"
)

type Options struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	URL        string
}

func run(opts *Options) error {
	resp, err := opts.HTTPClient.Get(opts.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// BAD: server-controlled bytes are written to IO.Out, which does not
	// sanitize. Any ANSI escape sequences in the response will be rendered
	// by the user's terminal.
	fmt.Fprint(opts.IO.Out, string(body))
	return nil
}
