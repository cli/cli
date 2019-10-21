package command

import (
	"fmt"

	"github.com/github/gh-cli/api"
	"github.com/spf13/cobra"
)

func init() {
	var cmd = &cobra.Command{
		Use:   "push",
		Short: "Push commits",
		Long:  `Transfers commits you made on your computer to GitHub.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return push(cmd, args)
		},
	}

	RootCmd.AddCommand(cmd)
}

func push(cmd *cobra.Command, args []string) error {
	fmt.Printf("🌭 %+v\n", "HI!")
	hasPermission, err := api.HasPushPermission()
	if err != nil {
		return err
	}

	if hasPermission {
		fmt.Printf("🌭 %+v\n", "HAS ACCESS")
	} else {
		fmt.Printf("🌭 %+v\n", "does not have access")
	}

	return nil
}
