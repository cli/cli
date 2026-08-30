<!-- Generated from unsanitized-response-to-terminal.qhelp via 'codeql generate query-help'. Regenerate after editing the .qhelp source. -->
# HTTP response content reaches a terminal without ContentOut or sanitization
Bytes consumed from an HTTP response body are server-controlled and may contain ANSI escape sequences. When those bytes reach a terminal writer without sanitization, a remote attacker can move the cursor, repaint the screen, fake a shell prompt, write to the clipboard via OSC sequences, or otherwise manipulate the user's terminal session.

This query flags HTTP response content, including bytes reintroduced by base64 decoding, that reaches a terminal writer (`IOStreams.Out`, `IOStreams.ErrOut`, `os.Stdout`, or `os.Stderr`) without first being written to `IOStreams.ContentOut`, sanitized, or decoded as a structured format (e.g. `encoding/json`).


## Recommendation
Choose the writer based on the kind of content you are printing:

* `IOStreams.Out` is for application output the developer authored: tables, prompts, formatted messages, color-coded status. It does not sanitize and never should, because the developer controls every byte that reaches it.
* `IOStreams.ContentOut` is for external content the developer did not author: HTTP response bodies, file contents fetched from a remote, anything where a third party chose the bytes. It sanitizes ANSI escape sequences by default.
These patterns satisfy the query:

1. Label the content at its source as `iostreams.Untrusted` and print it with `String()` (or any `fmt` verb, which calls `String()`); the value sanitizes itself. Its `Raw()` method is the explicit opt-out and is still flagged if it reaches a terminal.
1. Write external bytes to `IOStreams.ContentOut`.
1. Decode the bytes into a structured value first (`json.Unmarshal`, `(*json.Decoder).Decode`); the fields you print afterwards are no longer raw external content.
1. For commands where the user has opted into raw output, add a per-command `--allow-escape-sequences` flag and call `opts.IO.SetContentSanitization(false)` before writing. The bytes still go through `ContentOut`, but ContentOut becomes a passthrough for that invocation.

## Example
In the following BAD example, the response body is written directly to `IOStreams.Out`. A server can embed ANSI escape sequences in the response and they will be rendered by the user's terminal:


```go
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

```
In the following GOOD example, the same body is written to `IOStreams.ContentOut`, which sanitizes ANSI escape sequences. An `--allow-escape-sequences` flag is provided for users who explicitly want raw output for trusted content:


```go
package example

import (
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type Options struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	URL        string

	AllowEscapeSequences bool
}

func newCmd() *cobra.Command {
	opts := &Options{}
	cmd := &cobra.Command{
		Use:  "fetch",
		RunE: func(*cobra.Command, []string) error { return run(opts) },
	}
	cmd.Flags().BoolVar(&opts.AllowEscapeSequences, "allow-escape-sequences", false,
		"Allow printing terminal escape sequences")
	return cmd
}

func run(opts *Options) error {
	if opts.AllowEscapeSequences {
		opts.IO.SetContentSanitization(false)
	}

	resp, err := opts.HTTPClient.Get(opts.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// GOOD: external bytes flow through ContentOut, which sanitizes ANSI
	// escape sequences by default. The --allow-escape-sequences flag is the
	// documented opt-out for trusted content.
	fmt.Fprint(opts.IO.ContentOut, string(body))
	return nil
}

```
