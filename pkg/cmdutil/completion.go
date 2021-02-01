package cmdutil

import (
	"github.com/cli/cli/pkg/cmdutil/action"
	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

// InitCompletions finalizes completion configuration
// TODO move to root cmd
func InitCompletions(cmd *cobra.Command) {
	carapace.Gen(cmd)
	addAliasCompletion(cmd)
}

func addAliasCompletion(cmd *cobra.Command) {
	if c, _, err := cmd.Find([]string{"_carapace"}); err == nil {
		c.Annotations = map[string]string{
			"skipAuthCheck": "true",
		}
		c.PreRun = func(cmd *cobra.Command, args []string) {
			if aliases, err := action.Aliases(); err == nil {
				for key, value := range aliases {
					cmd.Root().AddCommand(&cobra.Command{Use: key, Short: value, Run: func(cmd *cobra.Command, args []string) {}})
				}
			}
		}
	}
}
