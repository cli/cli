package codespace

import (
	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/spf13/cobra"
)

func newSSHCmd(app *codespaces.App) *cobra.Command {
	var opts codespaces.SSHOptions

	sshCmd := &cobra.Command{
		Use:   "ssh [flags] [--] [ssh-flags] [command]",
		Short: "SSH into a codespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.SSH(cmd.Context(), args, opts)
		},
	}

	sshCmd.Flags().StringVarP(&opts.Profile, "profile", "", "", "Name of the SSH profile to use")
	sshCmd.Flags().IntVarP(&opts.ServerPort, "server-port", "", 0, "SSH server port number (0 => pick unused)")
	sshCmd.Flags().StringVarP(&opts.Codespace, "codespace", "c", "", "Name of the codespace")
	sshCmd.Flags().BoolVarP(&opts.Debug, "debug", "d", false, "Log debug data to a file")
	sshCmd.Flags().StringVarP(&opts.DebugFile, "debug-file", "", "", "Path of the file log to")

	return sshCmd
}
