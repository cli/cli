package Command

import "github.com/spf13/cobra"
package Command

func includeRepoFlag(cmd *cobra.Command) {
	cmd.StringP("repo", "R", "", "Select another repository using the `OWNER/REPO` format")
}
