package auth

import (
	authLoginCmd "github.com/cli/cli/pkg/cmd/auth/login"
	authLogoutCmd "github.com/cli/cli/pkg/cmd/auth/logout"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdAuth(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
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

	cmd.AddCommand(authLoginCmd.NewCmdLogin(f, nil))
	cmd.AddCommand(authLogoutCmd.NewCmdLogout(f, nil))

	return cmd
}
