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
		return fmt.Errorf("get or choose codespace: %w", err)
	}

	session, closeSession, err := startSession(ctx, codespace, a, opts.debug, opts.debugFile)
	if err != nil {
		return err
	}
	defer closeSession(&err)

	a.StartProgressIndicatorWithLabel("Fetching Jupyter Details")
	jupyterServerPort, jupyterServerUrl, err := session.StartJupyterServer(ctx)
	a.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("error getting jupyter server details: %w", err)
	}

	// Ensure local port is listening before client (Shell) connects.
	// The client picks a random port when jupyterDestPort is 0.
	jupyterDestPort := 0
	listen, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", jupyterDestPort))
	if err != nil {
		return err
	}
	defer listen.Close()
	jupyterDestPort = listen.Addr().(*net.TCPAddr).Port

	tunnelClosed := make(chan error, 1)
	go func() {
		fwd := liveshare.NewPortForwarder(session, "jupyter", jupyterServerPort, true)
		tunnelClosed <- fwd.ForwardToListener(ctx, listen) // always non-nil
	}()

	// Preserve the server URL's token
	targetUrl := strings.Replace(jupyterServerUrl, fmt.Sprintf("%d", jupyterServerPort), fmt.Sprintf("%d", jupyterDestPort), 1)
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
