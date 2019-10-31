package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(completionCmd)
	completionCmd.Flags().StringP("shell", "s", "bash", "The type of shell")
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
		shellType, err := cmd.Flags().GetString("shell")
		if err != nil {
			return err
		}

		switch shellType {
		case "bash":
			RootCmd.GenBashCompletion(cmd.OutOrStdout())
		case "zsh":
			RootCmd.GenZshCompletion(cmd.OutOrStdout())
		default:
			return fmt.Errorf("unsupported shell type: %s", shellType)
		}
		return nil
	},
}
