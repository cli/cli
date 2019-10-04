package command

import (
	"os"

	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(completionCmd)
}

var completionCmd = &cobra.Command{
	Use:    "completion",
	Hidden: true,
	Short:  "Generates bash completion script",
	Long: `To enable completion in your shell, run:

  eval "$(gh completion)"

You can add that to your '~/.bash_profile' to enable completion whenever you
start a new shell.
`,
	Run: func(cmd *cobra.Command, args []string) {
		RootCmd.GenBashCompletion(os.Stdout)
	},
}
