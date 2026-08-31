package codespace

import (
	"context"
	"fmt"
	"net/url"

	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/internal/codespaces/portforwarder"
	"github.com/cli/cli/v2/internal/codespaces/rpc"
	"github.com/microsoft/dev-tunnels/go/tunnels"
	"github.com/spf13/cobra"
)

func newJupyterCmd(app *App) *cobra.Command {
	var selector *CodespaceSelector

	jupyterCmd := &cobra.Command{
		Use:   "jupyter",
		Short: "Open a codespace in JupyterLab",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Jupyter(cmd.Context(), selector)
		},
	}

	selector = AddCodespaceSelector(jupyterCmd, app.apiClient)

	return jupyterCmd
}

func (a *App) Jupyter(ctx context.Context, selector *CodespaceSelector) (err error) {
	// Ensure all child tasks (e.g. port forwarding) terminate before return.
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

	var (
		invoker    rpc.Invoker
		serverPort int
		serverUrl  string
	)
	err = a.RunWithProgress("Starting JupyterLab on codespace", func() (err error) {
		invoker, err = rpc.CreateInvoker(ctx, fwd)
		if err != nil {
			return
		}

		serverPort, serverUrl, err = invoker.StartJupyterServer(ctx)
		return
	})
	if invoker != nil {
		defer safeClose(invoker, &err)
	}
	if err != nil {
		return err
	}

	// Forward the port so that it is reachable over the tunnel's web forwarding origin.
	// Visibility is deliberately left unset, which Dev Tunnels treats as private for a
	// newly created port. A port that already exists keeps its current access control,
	// because createTunnelPort ignores the resulting conflict, so the visibility of the
	// port is verified below rather than assumed.
	opts := portforwarder.ForwardPortOpts{
		Port: serverPort,
	}
	if err := fwd.ForwardPort(ctx, opts); err != nil {
		return fmt.Errorf("failed to forward JupyterLab port: %w", err)
	}

	targetURL, err := jupyterWebForwardingURL(ctx, fwd, serverPort, serverUrl)
	if err != nil {
		return err
	}

	err = a.browser.Browse(targetURL)
	if err != nil {
		return fmt.Errorf("failed to open JupyterLab in browser: %w", err)
	}

	fmt.Fprintln(a.io.Out, targetURL)

	// The browser reaches JupyterLab over the tunnel's web forwarding origin rather than
	// through this process, so there is no local listener left to wait on. Block until the
	// user interrupts, which keeps the port forwarded for the lifetime of the command.
	<-ctx.Done()

	return nil
}

// jupyterWebForwardingURL builds the URL to open for the forwarded JupyterLab port.
//
// The origin is taken from the tunnel's own web forwarding URI, never from the
// codespace. Only the path and query of the codespace-reported server URL are reused,
// because those carry the JupyterLab auth token. The codespace is untrusted here: it
// supplies both the port and the server URL, so previously it could choose the origin
// that its own content was rendered on in the user's browser.
//
// The origin comes from the port's own PortForwardingURIs rather than from the
// name-and-port pattern used by portInfo.BrowseURL, because the tunnel service generates
// it for the host that is actually serving the port. That keeps the URL correct when the
// forwarding domain is not the default one, such as on GitHub Enterprise Server.
//
// The port must be private. Forwarding does not change the access control of a port
// that already exists, so a port left public by an earlier forward, or by the codespace
// itself, would otherwise expose the JupyterLab auth token on a publicly reachable URL.
func jupyterWebForwardingURL(ctx context.Context, fwd portforwarder.PortForwarder, serverPort int, serverURL string) (string, error) {
	ports, err := fwd.ListPorts(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list forwarded ports: %w", err)
	}

	var forwardingURI string
	for _, port := range ports {
		if int(port.PortNumber) != serverPort {
			continue
		}
		var entries []tunnels.TunnelAccessControlEntry
		if port.AccessControl != nil {
			entries = port.AccessControl.Entries
		}
		if visibility := portforwarder.AccessControlEntriesToVisibility(entries); visibility != portforwarder.PrivatePortVisibility {
			return "", fmt.Errorf("JupyterLab port %d is %s rather than private, so it was not opened. Set the port to private and try again", serverPort, visibility)
		}
		if len(port.PortForwardingURIs) > 0 {
			forwardingURI = port.PortForwardingURIs[0]
		}
		break
	}

	if forwardingURI == "" {
		return "", fmt.Errorf("no web forwarding URL available for JupyterLab port %d", serverPort)
	}

	base, err := url.Parse(forwardingURI)
	if err != nil {
		return "", fmt.Errorf("failed to parse web forwarding URL: %w", err)
	}

	reported, err := url.Parse(serverURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse JupyterLab server URL: %w", err)
	}

	// Resolving a reference keeps the forwarding URI's scheme and host while taking the
	// path, query and fragment from the reported URL.
	target := base.ResolveReference(&url.URL{
		Path:     reported.Path,
		RawQuery: reported.RawQuery,
		Fragment: reported.Fragment,
	})

	return target.String(), nil
}
