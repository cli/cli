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

	// Forward the port so it is reachable over the tunnel's web forwarding origin.
	// Leaving visibility unset makes Dev Tunnels create a private port, but an existing
	// port keeps whatever access control it already has, because createTunnelPort
	// ignores the resulting conflict. jupyterWebForwardingURL therefore reads the
	// visibility back and verifies it.
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

	// Block until the user interrupts, holding this command's connection to the codespace
	// open in the meantime.
	//
	// Exiting closes that connection but does not unregister the tunnel port or stop
	// JupyterLab, and the browser reaches JupyterLab through the tunnel service rather than
	// through this process. Exiting therefore does not end browser access, for as long as
	// the codespace and JupyterLab remain available. The previous implementation also left
	// the tunnel port registered; what changed is that the browser now holds an address that
	// does not depend on this process, instead of a local one that died with it.
	<-ctx.Done()

	return nil
}

// jupyterWebForwardingURL builds the URL to open for the forwarded JupyterLab port.
//
// The origin comes from the tunnel's own web forwarding URI. The codespace supplies only
// the path, query and fragment, which carry the JupyterLab auth token. The codespace is
// untrusted here: it reports both the port and the server URL, so letting it supply the
// origin would let it choose the origin its own content runs on in the user's browser.
//
// Reading the origin from the port's PortForwardingURIs keeps the URL correct wherever the
// forwarding domain differs from the default, such as on GitHub Enterprise Server, because
// the tunnel service generates that URI for the host actually serving the port. This is why
// it is preferred over the name-and-port pattern in portInfo.BrowseURL.
//
// The port must be private, as it is when the tunnel service creates it here. Forwarding an
// existing port leaves its access control unchanged, so a port left public by an earlier
// forward, or by the codespace itself, would otherwise put the JupyterLab auth token on a
// URL that is reachable without the private-port authentication gate. The check reflects the
// port's visibility at the moment the URL is built and does not constrain it afterwards.
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
