package codespace

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/pkg/liveshare"
	"github.com/spf13/cobra"
)

type jupyterOptions struct {
	codespace string
	debug     bool
	debugFile string
	stdio     bool
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
	// TODO: Whatever it takes to call StartJupyterServer
	// That returns the port and server url

	// TODO: Share this code with ssh.go and logs.go
	// We're all doing the same thing: starting agent session

	// Ensure all child tasks (e.g. port forwarding) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	codespace, err := getOrChooseCodespace(ctx, a.apiClient, opts.codespace)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %w", err)
	}

	// While connecting, ensure in the background that the user has keys installed.
	// That lets us report a more useful error message if they don't.
	authkeys := make(chan error, 1)
	go func() {
		authkeys <- checkAuthorizedKeys(ctx, a.apiClient)
	}()

	liveshareLogger := noopLogger()
	if opts.debug {
		debugLogger, err := newFileLogger(opts.debugFile)
		if err != nil {
			return fmt.Errorf("error creating debug logger: %w", err)
		}
		defer safeClose(debugLogger, &err)

		liveshareLogger = debugLogger.Logger
		a.errLogger.Printf("Debug file located at: %s", debugLogger.Name())
	}

	session, err := codespaces.ConnectToLiveshare(ctx, a, liveshareLogger, a.apiClient, codespace)
	if err != nil {
		if authErr := <-authkeys; authErr != nil {
			return authErr
		}
		return fmt.Errorf("error connecting to codespace: %w", err)
	}
	defer safeClose(session, &err)

	a.StartProgressIndicatorWithLabel("Fetching Jupyter Details")
	remoteJupyterPort, jupyterServerUrl, err := session.StartJupyterServer(ctx)
	a.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("error getting jupyter server details: %w", err)
	}

	localJupyterServerPort := 0
	// localSSHServerPort := opts.serverPort
	// usingCustomPort := localSSHServerPort != 0 // suppress log of command line in Shell

	// Ensure local port is listening before client (Shell) connects.
	// Unless the user specifies a server port, localSSHServerPort is 0
	// and thus the client will pick a random port.
	listen, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localJupyterServerPort))
	if err != nil {
		return err
	}
	defer listen.Close()
	localJupyterServerPort = listen.Addr().(*net.TCPAddr).Port

	tunnelClosed := make(chan error, 1)
	go func() {
		fwd := liveshare.NewPortForwarder(session, "jupyter", remoteJupyterPort, true)

		// TODO: Cancel context when browser closes
		tunnelClosed <- fwd.ForwardToListener(ctx, listen) // always non-nil
	}()

	// Launch browser and connect to JupyterLab
	targetUrl := strings.Replace(jupyterServerUrl, fmt.Sprintf("%d", remoteJupyterPort), fmt.Sprintf("%d", localJupyterServerPort), 1)
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
