package codespace

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/cli/cli/v2/internal/codespaces/grpc"
	"github.com/cli/cli/v2/pkg/liveshare"
	"github.com/spf13/cobra"
)

func newJupyterCmd(app *App) *cobra.Command {
	var codespace string

	jupyterCmd := &cobra.Command{
		Use:   "jupyter",
		Short: "Open a codespace in JupyterLab",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Jupyter(cmd.Context(), codespace)
		},
	}

	jupyterCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")

	return jupyterCmd
}

func (a *App) Jupyter(ctx context.Context, codespaceName string) (err error) {
	// Ensure all child tasks (e.g. port forwarding) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	codespace, err := getOrChooseCodespace(ctx, a.apiClient, codespaceName)
	if err != nil {
		return err
	}

	session, err := startLiveShareSession(ctx, codespace, a, false, "")
	if err != nil {
		return err
	}
	defer safeClose(session, &err)

	a.StartProgressIndicatorWithLabel("Starting JupyterLab on codespace")
	client, err := connectToGRPCServer(ctx, session, codespace.Connection.SessionToken)
	if err != nil {
		return fmt.Errorf("failed to connect to internal server: %w", err)
	}
	defer safeClose(client, &err)

	serverPort, serverUrl, err := startJupyterServer(ctx, client)
	if err != nil {
		return fmt.Errorf("failed to start JupyterLab server: %w", err)
	}
	a.StopProgressIndicator()

	// Pass 0 to pick a random port
	listen, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", 0))
	if err != nil {
		return err
	}
	defer listen.Close()
	destPort := listen.Addr().(*net.TCPAddr).Port

	tunnelClosed := make(chan error, 1)
	go func() {
		fwd := liveshare.NewPortForwarder(session, "jupyter", serverPort, true)
		tunnelClosed <- fwd.ForwardToListener(ctx, listen) // always non-nil
	}()

	// Server URL contains an authentication token that must be preserved
	targetUrl := strings.Replace(serverUrl, fmt.Sprintf("%d", serverPort), fmt.Sprintf("%d", destPort), 1)
	err = a.browser.Browse(targetUrl)
	if err != nil {
		return fmt.Errorf("failed to open JupyterLab in browser: %w", err)
	}

	fmt.Fprintln(a.io.Out, targetUrl)

	select {
	case err := <-tunnelClosed:
		return fmt.Errorf("tunnel closed: %w", err)
	case <-ctx.Done():
		return nil // success
	}
}

func connectToGRPCServer(ctx context.Context, session liveshareSession, token string) (*grpc.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, grpc.ConnectionTimeout)
	defer cancel()

	client, err := grpc.Connect(ctx, session, token)
	if err != nil {
		return nil, fmt.Errorf("error connecting to internal server: %w", err)
	}

	return client, nil
}

func startJupyterServer(ctx context.Context, client *grpc.Client) (int, string, error) {
	ctx, cancel := context.WithTimeout(ctx, grpc.RequestTimeout)
	defer cancel()

	serverPort, serverUrl, err := client.StartJupyterServer(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("failed to start JupyterLab server: %w", err)
	}

	return serverPort, serverUrl, nil
}
