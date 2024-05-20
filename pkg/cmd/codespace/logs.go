package codespace

import (
	"context"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/internal/codespaces/portforwarder"
	"github.com/cli/cli/v2/internal/codespaces/rpc"
	"github.com/spf13/cobra"
)

func newLogsCmd(app *App) *cobra.Command {
	var (
		selector *CodespaceSelector
		follow   bool
	)

	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Access codespace logs",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Logs(cmd.Context(), selector, follow)
		},
	}

	selector = AddCodespaceSelector(logsCmd, app.apiClient)

	logsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "Tail and follow the logs")

	return logsCmd
}

func (a *App) Logs(ctx context.Context, selector *CodespaceSelector, follow bool) (err error) {
	// Ensure all child tasks (port forwarding, remote exec) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	codespace, err := selector.Select(ctx)
	if err != nil {
		return err
	}

	codespaceConnection, err := codespaces.GetCodespaceConnection(ctx, a, a.apiClient, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to codespace: %w", err)
	}

	fwd, err := portforwarder.NewPortForwarder(ctx, codespaceConnection)
	if err != nil {
		return fmt.Errorf("failed to create port forwarder: %w", err)
	}
	defer safeClose(fwd, &err)

	// Ensure local port is listening before client (getPostCreateOutput) connects.
	listen, localPort, err := codespaces.ListenTCP(0, false)
	if err != nil {
		return err
	}
	defer listen.Close()

	remoteSSHServerPort, sshUser := 0, ""
	err = a.RunWithProgress("Fetching SSH Details", func() (err error) {
		invoker, err := rpc.CreateInvoker(ctx, fwd)
		if err != nil {
			return
		}
		defer safeClose(invoker, &err)

		remoteSSHServerPort, sshUser, err = invoker.StartSSHServer(ctx)
		return
	})
	if err != nil {
		return fmt.Errorf("error getting ssh server details: %w", err)
	}

	cmdType := "cat"
	if follow {
		cmdType = "tail -f"
	}

	dst := fmt.Sprintf("%s@localhost", sshUser)
	cmd, err := codespaces.NewRemoteCommand(
		ctx, localPort, dst, fmt.Sprintf("%s /workspaces/.codespaces/.persistedshare/creation.log", cmdType),
	)
	if err != nil {
		return fmt.Errorf("remote command: %w", err)
	}

	tunnelClosed := make(chan error, 1)
	go func() {
		opts := portforwarder.ForwardPortOpts{
			Port:     remoteSSHServerPort,
			Internal: true,
		}
		tunnelClosed <- fwd.ForwardPortToListener(ctx, opts, listen)
	}()

	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.Run()
	}()

	select {
	case err := <-tunnelClosed:
		return fmt.Errorf("connection closed: %w", err)
	case err := <-cmdDone:
		if err != nil {
			return fmt.Errorf("error retrieving logs: %w", err)
		}

		return nil // success
	}
}
