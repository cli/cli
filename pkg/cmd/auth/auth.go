package auth

import (
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "auth <command>",
	Short: "Login, logout, and refresh your authentication",
	Long:  `Manage gh's authentication state.`,
	// TODO this all doesn't exist yet
	//Example: heredoc.Doc(`
	//	$ gh auth login
	//	$ gh auth status
	//	$ gh auth refresh --scopes gist
	//	$ gh auth logout
	//`),
}
