package completion

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
		Use:   "completion -s <shell>",
		Short: "Generate shell completion scripts",
		Long: heredoc.Docf(`
			Generate shell completion scripts for GitHub CLI commands.

			When installing GitHub CLI through a package manager, it's possible that
			no additional shell configuration is necessary to gain completion support. For
			Homebrew, see <https://docs.brew.sh/Shell-Completion>

			If you need to set up completions manually, follow the instructions below. The exact
			config file locations might vary based on your system. Make sure to restart your
			shell before testing whether completions are working.

			### bash

			First, ensure that you install %[1]sbash-completion%[1]s using your package manager.

			After, add this to your %[1]s~/.bash_profile%[1]s:

				eval "$(gh completion -s bash)"
			
			### zsh

			Generate a %[1]s_gh%[1]s completion script and put it somewhere in your %[1]s$fpath%[1]s:

				gh completion -s zsh > /usr/local/share/zsh/site-functions/_gh

			Ensure that the following is present in your %[1]s~/.zshrc%[1]s:

				autoload -U compinit
				compinit -i
			
			Zsh version 5.7 or later is recommended.

			### fish

			Generate a %[1]sgh.fish%[1]s completion script:

				gh completion -s fish > ~/.config/fish/completions/gh.fish
		`, "`"),
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
				return rootCmd.GenBashCompletionV2(w, true)
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
		DisableFlagsInUseLine: true,
	}

	cmdutil.DisableAuthCheck(cmd)

	cmd.Flags().StringVarP(&shellType, "shell", "s", "", "Shell type: {bash|zsh|fish|powershell}")

	return cmd
}
