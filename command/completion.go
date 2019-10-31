package command

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var shellType string

func init() {
	RootCmd.AddCommand(completionCmd)
	completionCmd.Flags().StringVarP(&shellType, "shell", "s", "bash", "The type of shell")
}

var completionCmd = &cobra.Command{
	Use:    "completion",
	Hidden: true,
	Short:  "Generates completion scripts",
	Long: `To enable completion in your shell, run:

  eval "$(gh completion)"

You can add that to your '~/.bash_profile' to enable completion whenever you
start a new shell.

When installing with Homebrew, see https://docs.brew.sh/Shell-Completion
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch shellType {
		case "bash":
			RootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			RootCmd.GenZshCompletion(os.Stdout)
		default:
			return fmt.Errorf("unsupported shell type: %s", shellType)
		}
		return nil
	},
}
