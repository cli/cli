package auth

import (
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	gitCredentialCmd "github.com/cli/cli/v2/pkg/cmd/auth/gitcredential"
	authLoginCmd "github.com/cli/cli/v2/pkg/cmd/auth/login"
	authLogoutCmd "github.com/cli/cli/v2/pkg/cmd/auth/logout"
	authRefreshCmd "github.com/cli/cli/v2/pkg/cmd/auth/refresh"
	authSetupGitCmd "github.com/cli/cli/v2/pkg/cmd/auth/setupgit"
	authStatusCmd "github.com/cli/cli/v2/pkg/cmd/auth/status"
	authSwitchCmd "github.com/cli/cli/v2/pkg/cmd/auth/switch"
	authTokenCmd "github.com/cli/cli/v2/pkg/cmd/auth/token"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdAuth(f *cmdutil.Factory, telemetry ghtelemetry.InvocationRecorder) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "auth <command>",
		Short:   "Authenticate gh and git with GitHub",
		GroupID: "core",
	}

	cmdutil.DisableAuthCheck(cmd)

	loginCmd := authLoginCmd.NewCmdLogin(f, telemetry, nil)
	refreshCmd := authRefreshCmd.NewCmdRefresh(f, telemetry, nil)

	cmd.AddCommand(loginCmd)
	cmd.AddCommand(authLogoutCmd.NewCmdLogout(f, nil))
	cmd.AddCommand(authStatusCmd.NewCmdStatus(f, nil))
	cmd.AddCommand(refreshCmd)
	cmd.AddCommand(gitCredentialCmd.NewCmdCredential(f, nil))
	cmd.AddCommand(authSetupGitCmd.NewCmdSetupGit(f, nil))
	cmd.AddCommand(authTokenCmd.NewCmdToken(f, nil))
	cmd.AddCommand(authSwitchCmd.NewCmdSwitch(f, nil))

	cmdutil.DisableTelemetryForSubcommands(cmd)

	return cmd
}
