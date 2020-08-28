package auth

import (
	authLoginCmd "github.com/cli/cli/pkg/cmd/auth/login"
	authLogoutCmd "github.com/cli/cli/pkg/cmd/auth/logout"
	authRefreshCmd "github.com/cli/cli/pkg/cmd/auth/refresh"
	authStatusCmd "github.com/cli/cli/pkg/cmd/auth/status"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdAuth(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth <command>",
		Short: "Login, logout, and refresh your authentication",
		Long:  `Manage gh's authentication state.`,
	}

	cmdutil.DisableAuthCheck(cmd)

	cmd.AddCommand(authLoginCmd.NewCmdLogin(f, nil))
	cmd.AddCommand(authLogoutCmd.NewCmdLogout(f, nil))
	cmd.AddCommand(authStatusCmd.NewCmdStatus(f, nil))
	cmd.AddCommand(authRefreshCmd.NewCmdRefresh(f, nil))

	return cmd
}
