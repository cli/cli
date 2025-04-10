package codespace

import (
	"context"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/spf13/cobra"
)

func newLogsCmd(app *App) *cobra.Command {
	var (
		selector *CodespaceSelector
		follow   bool
		profile  string
	)

	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Access codespace logs",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Logs(cmd.Context(), selector, follow, profile)
		},
	}

	selector = AddCodespaceSelector(logsCmd, app.apiClient)

	logsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "Tail and follow the logs")
	logsCmd.Flags().StringVarP(&profile, "profile", "", "", "Name of the SSH profile to use")

	return logsCmd
}

func (a *App) Logs(ctx context.Context, selector *CodespaceSelector, follow bool, profile string) (err error) {
	// Ensure all child tasks (port forwarding, remote exec) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logCmd := "cat"
	if follow {
		logCmd = "tail -f"
	}

	args := []string{fmt.Sprintf("%s /workspaces/.codespaces/.persistedshare/creation.log", logCmd)}

	tunnel, args, err := a.connect(ctx, selector, args, profile)
	if err != nil {
		return err
	}
	defer tunnel.Close()

	// The serverPort 0 means it will pick an unused port randomly as we don't
	// have a `server-port` argument as we do with the `ssh` command.
	if err := tunnel.Forward(ctx, 0); err != nil {
		return fmt.Errorf("error forwarding tunnel: %w", err)
	}

	return tunnel.Run(profile, func(destination string, port int) error {
		cmd, err := codespaces.NewRemoteCommand(ctx, port, destination, args...)
		if err != nil {
			return fmt.Errorf("error retrieving logs: %w", err)
		}
		return cmd.Run()
	})
}
