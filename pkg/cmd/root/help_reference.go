package root

import (
	"fmt"
	"strings"

	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/text"
	"github.com/spf13/cobra"
)

func referenceHelpFn(cmd *cobra.Command, io *iostreams.IOStreams) func() string {
	cs := io.ColorScheme()
	return func() string {
		reftext := fmt.Sprintf("%s reference\n\n", cs.Bold("gh"))

		for _, c := range cmd.Commands() {
			reftext += cmdRef(cs, c, 0)
		}
		return reftext
	}
}

func cmdRef(cs *iostreams.ColorScheme, cmd *cobra.Command, lvl int) string {
	ref := ""

	if cmd.Hidden {
		return ref
	}

	cmdColor := cs.Bold
	if lvl > 0 {
		cmdColor = cs.Cyan
	}

	// Name + Description
	ref += fmt.Sprintf("%s%s  %s\n", strings.Repeat("  ", lvl), cmdColor(cmd.Use), cmd.Short)

	// Flags
	ref += text.Indent(dedent(cmd.Flags().FlagUsages()), strings.Repeat("  ", lvl+1))
	ref += text.Indent(dedent(cmd.PersistentFlags().FlagUsages()), strings.Repeat("  ", lvl+1))

	if cmd.HasPersistentFlags() || cmd.HasFlags() || cmd.HasAvailableSubCommands() {
		ref += "\n"
	}

	// Subcommands
	subcommands := cmd.Commands()
	for _, c := range subcommands {
		ref += cmdRef(cs, c, lvl+1)
	}

	if len(subcommands) == 0 {
		ref += "\n"
	}

	return ref
}
