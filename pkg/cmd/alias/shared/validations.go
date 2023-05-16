package shared

import (
	"strings"

	"github.com/google/shlex"
	"github.com/spf13/cobra"
)

// ValidAliasNameFunc returns a function that will check if the given string
// is a valid alias name. A name can be invalid if it shadows an existing
// command or it is nested under a command that does not exist.
func ValidAliasNameFunc(cmd *cobra.Command) func(string) bool {
	return func(args string) bool {
		split, err := shlex.Split(args)
		if err != nil || len(split) == 0 {
			return false
		}

		rootCmd := cmd.Root()
		foundCmd, foundArgs, _ := rootCmd.Find(split)
		// If there are no found args the user is trying to define an alias
		// that shadows an existing command.
		if foundCmd != nil && len(foundArgs) == 1 {
			return true
		}

		return false
	}
}

// ValidAliasExpansionFunc returns a function that will check if the given string
// is a valid alias expansion. All shell expansions are valid. Non-shell expansions
// are valid if they correspond to an existing command, extension, or alias.
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
