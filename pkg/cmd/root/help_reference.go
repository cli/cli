package root

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/spf13/cobra"
)

// longPager provides a pager over a commands Long message.
// It is currently only used for the reference command
func longPager(io *iostreams.IOStreams) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		wrapWidth := 0
		if io.IsStdoutTTY() {
			io.DetectTerminalTheme()
			wrapWidth = io.TerminalWidth()
		}

		md, err := markdown.Render(cmd.Long,
			markdown.WithTheme(io.TerminalTheme()),
			markdown.WithWrap(wrapWidth))
		if err != nil {
			fmt.Fprintln(io.ErrOut, err)
			return
		}

		if !io.IsStdoutTTY() {
			fmt.Fprint(io.Out, dedent(md))
			return
		}

		_ = io.StartPager()
		defer io.StopPager()
		fmt.Fprint(io.Out, md)
	}
}

func stringifyReference(cmd *cobra.Command) string {
	buf := bytes.NewBufferString("# gh reference\n\n")
	for _, c := range cmd.Commands() {
		if c.Hidden {
			continue
		}
		cmdRef(buf, c, 2)
	}
	return buf.String()
}

func cmdRef(w io.Writer, cmd *cobra.Command, depth int) {
	// Name + Description
	fmt.Fprintf(w, "%s `%s`\n\n", strings.Repeat("#", depth), cmd.UseLine())
	fmt.Fprintf(w, "%s\n\n", cmd.Short)

	// Flags
	// TODO: fold in InheritedFlags/PersistentFlags, but omit `--help` due to repetitiveness
	if flagUsages := cmd.Flags().FlagUsages(); flagUsages != "" {
		fmt.Fprintf(w, "```\n%s````\n\n", dedent(flagUsages))
	}

	// Aliases
	if len(cmd.Aliases) > 0 {
		fmt.Fprintf(w, "%s\n\n", "Aliases")
		fmt.Fprintf(w, "\n%s\n\n", dedent(strings.Join(BuildAliasList(cmd, cmd.Aliases), ", ")))
	}

	// Subcommands
	for _, c := range cmd.Commands() {
		if c.Hidden {
			continue
		}
		cmdRef(w, c, depth+1)
	}
}
