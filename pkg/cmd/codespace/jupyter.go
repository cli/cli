package codespace

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/cli/cli/v2/pkg/liveshare"
	"github.com/spf13/cobra"
)

type jupyterOptions struct {
	codespace string
	debug     bool
	debugFile string
}

func newJupyterCmd(app *App) *cobra.Command {
	var opts jupyterOptions

	jupyterCmd := &cobra.Command{
		Use:   "jupyter",
		Short: "Open a codespace in JupyterLab",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Jupyter(cmd.Context(), opts)
		},
	}

	return jupyterCmd
}

func (a *App) Jupyter(ctx context.Context, opts jupyterOptions) error {
	// Ensure all child tasks (e.g. port forwarding) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	codespace, err := getOrChooseCodespace(ctx, a.apiClient, opts.codespace)
	if err != nil {
		return err
	}

	session, closeSession, err := startSession(ctx, codespace, a, opts.debug, opts.debugFile)
	if err != nil {
		return err
	}
	defer closeSession(&err)

	a.StartProgressIndicatorWithLabel("Starting JupyterLab on codespace")
	serverPort, serverUrl, err := session.StartJupyterServer(ctx)
	a.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("error getting jupyter server details: %w", err)
	}

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
		return err
	}

	select {
	case err := <-tunnelClosed:
		return fmt.Errorf("tunnel closed: %w", err)
	case <-ctx.Done():
		return nil // success
	}
}
