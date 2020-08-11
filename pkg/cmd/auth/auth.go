package auth

import (
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "auth <command>",
	Short: "Login, logout, and refresh your authentication",
	Long:  `Manage gh's authentication state.`,
}
