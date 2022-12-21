package auth

import (
	gitCredentialCmd "github.com/cli/cli/v2/pkg/cmd/auth/gitcredential"
	authLoginCmd "github.com/cli/cli/v2/pkg/cmd/auth/login"
	authLogoutCmd "github.com/cli/cli/v2/pkg/cmd/auth/logout"
	authRefreshCmd "github.com/cli/cli/v2/pkg/cmd/auth/refresh"
	authSetupGitCmd "github.com/cli/cli/v2/pkg/cmd/auth/setupgit"
	authStatusCmd "github.com/cli/cli/v2/pkg/cmd/auth/status"
	authTokenCmd "github.com/cli/cli/v2/pkg/cmd/auth/token"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdAuth(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "auth <command>",
		Short:   "Authenticate gh and git with GitHub",
		GroupID: "core",
	}

	cmdutil.DisableAuthCheck(cmd)

	cmd.AddCommand(authLoginCmd.NewCmdLogin(f, nil))
	cmd.AddCommand(authLogoutCmd.NewCmdLogout(f, nil))
	cmd.AddCommand(authStatusCmd.NewCmdStatus(f, nil))
	cmd.AddCommand(authRefreshCmd.NewCmdRefresh(f, nil))
	cmd.AddCommand(gitCredentialCmd.NewCmdCredential(f, nil))
	cmd.AddCommand(authSetupGitCmd.NewCmdSetupGit(f, nil))
	cmd.AddCommand(authTokenCmd.NewCmdToken(f, nil))

	return cmd
}
