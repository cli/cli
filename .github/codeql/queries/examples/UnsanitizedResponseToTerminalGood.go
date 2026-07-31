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
