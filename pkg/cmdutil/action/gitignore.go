package action

import (
	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

func ActionGitignoreTemplates(cmd *cobra.Command) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		var queryResult []string
		return ApiV3Action(cmd, `gitignore/templates`, &queryResult, func() carapace.Action {
			return carapace.ActionValues(queryResult...)
		})
	})
}
