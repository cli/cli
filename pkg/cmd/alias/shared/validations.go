package shared

import (
	"strings"

	"github.com/google/shlex"
	"github.com/spf13/cobra"
)

// ValidAliasNameFunc returns a function that will check if the given string
// is a valid alias name. A name is valid if:
//   - it does not shadow an existing command,
//   - it is not nested under a command that is runnable,
//   - it is not nested under a command that does not exist.
func ValidAliasNameFunc(cmd *cobra.Command) func(string) bool {
	return func(args string) bool {
		split, err := shlex.Split(args)
		if err != nil || len(split) == 0 {
			return false
		}

		rootCmd := cmd.Root()
		foundCmd, foundArgs, _ := rootCmd.Find(split)
		if foundCmd != nil && !foundCmd.Runnable() && len(foundArgs) == 1 {
			return true
		}

		return false
	}
}

// ValidAliasExpansionFunc returns a function that will check if the given string
// is a valid alias expansion. An expansion is valid if:
//   - it is a shell expansion,
//   - it is a non-shell expansion that corresponds to an existing command, extension, or alias.
func ValidAliasExpansionFunc(cmd *cobra.Command) func(string) bool {
	return func(expansion string) bool {
		if strings.HasPrefix(expansion, "!") {
			return true
		}

		split, err := shlex.Split(expansion)
		if err != nil || len(split) == 0 {
			return false
		}

		rootCmd := cmd.Root()
		cmd, _, _ = rootCmd.Find(split)
		return cmd != rootCmd
	}
}
