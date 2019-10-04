package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "gh",
	Short: "GitHub CLI",
	Long:  `Do things with GitHub from your terminal`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("root")
	},
}
