package root

import (
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

func NewCmdCompletion(io *iostreams.IOStreams) *cobra.Command {
	var shellType string

	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
		Long: heredoc.Doc(`
			Generate shell completion scripts for GitHub CLI commands.

			The output of this command will be computer code and is meant to be saved to a
			file or immediately evaluated by an interactive shell.

			For example, for bash you could add this to your '~/.bash_profile':

				eval "$(gh completion -s bash)"

			When installing GitHub CLI through a package manager, however, it's possible that
			no additional shell configuration is necessary to gain completion support. For
			Homebrew, see https://docs.brew.sh/Shell-Completion
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if shellType == "" {
				if io.IsStdoutTTY() {
					return &cmdutil.FlagError{Err: errors.New("error: the value for `--shell` is required")}
				}
				shellType = "bash"
			}

			w := io.Out
			rootCmd := cmd.Parent()

			switch shellType {
			case "bash":
				return rootCmd.GenBashCompletion(w)
			case "zsh":
				return rootCmd.GenZshCompletion(w)
			case "powershell":
				return rootCmd.GenPowerShellCompletion(w)
			case "fish":
				return rootCmd.GenFishCompletion(w, true)
			default:
				return fmt.Errorf("unsupported shell type %q", shellType)
			}
		},
	}

	cmdutil.DisableAuthCheck(cmd)

	cmd.Flags().StringVarP(&shellType, "shell", "s", "", "Shell type: {bash|zsh|fish|powershell}")

	return cmd
}
