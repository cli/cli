package root

import (
	"fmt"
	"strings"

	"github.com/cli/cli/pkg/text"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func ReferenceHelpTopic(command *cobra.Command) *cobra.Command {
	command.Printf("%s reference\n\n", utils.Bold("gh"))

	for _, c := range command.Commands() {
		command.Print(cmdRef(c, 0))
	}

	ref := &cobra.Command{
		Use:    "reference",
		Hidden: true,
		Args:   cobra.NoArgs,
		Run:    helpTopicHelpFunc,
	}

	ref.SetHelpFunc(helpTopicHelpFunc)
	ref.SetUsageFunc(helpTopicUsageFunc)

	return ref
}

func cmdRef(cmd *cobra.Command, lvl int) string {
	ref := ""

	if cmd.Hidden {
		return ref
	}

	cmdColor := utils.Bold
	if lvl > 0 {
		cmdColor = utils.Cyan
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
		ref += cmdRef(c, lvl+1)
	}

	if len(subcommands) == 0 {
		ref += "\n"
	}

	return ref
}
