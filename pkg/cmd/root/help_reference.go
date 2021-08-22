package root

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/markdown"
	"github.com/spf13/cobra"
)

func referenceHelpFn(io *iostreams.IOStreams) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		wrapWidth := 0
		style := "notty"
		if io.IsStdoutTTY() {
			wrapWidth = io.TerminalWidth()
			style = markdown.GetStyle(io.DetectTerminalTheme())
		}

		md, err := markdown.RenderWithWrap(cmd.Long, style, wrapWidth)
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

func referenceLong(cmd *cobra.Command) string {
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

	// Subcommands
	for _, c := range cmd.Commands() {
		if c.Hidden {
			continue
		}
		cmdRef(w, c, depth+1)
	}
}
