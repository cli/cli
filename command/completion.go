package command

import (
	"fmt"

	"github.com/cli/cli/internal/cobrafish"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(completionCmd)
	completionCmd.Flags().StringP("shell", "s", "bash", "Shell type: {bash|zsh|fish|powershell}")
}

var completionCmd = &cobra.Command{
	Use:   "completion",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for GitHub CLI commands.

For example, for bash you could add this to your '~/.bash_profile':

	eval "$(gh completion)"

When installing GitHub CLI through a package manager, however, it's possible that
no additional shell configuration is necessary to gain completion support. For
Homebrew, see <https://docs.brew.sh/Shell-Completion>
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		shellType, err := cmd.Flags().GetString("shell")
		if err != nil {
			return err
		}

		switch shellType {
		case "bash":
			return RootCmd.GenBashCompletion(cmd.OutOrStdout())
		case "zsh":
			return RootCmd.GenZshCompletion(cmd.OutOrStdout())
		case "powershell":
			return RootCmd.GenPowerShellCompletion(cmd.OutOrStdout())
		case "fish":
			return cobrafish.GenCompletion(RootCmd, cmd.OutOrStdout())
		default:
			return fmt.Errorf("unsupported shell type %q", shellType)
		}
	},
}
