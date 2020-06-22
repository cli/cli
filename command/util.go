package command

import "github.com/spf13/cobra"

func includeRepoFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("repo", "R", "", "Select another repository using the `OWNER/REPO` format")
}
