package codespace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/liveshare"
	"github.com/cli/cli/v2/utils"
	"github.com/muhammadmuzzammil1998/jsonc"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

const (
	vscodeServerPortName       = "VSCodeServerInternal"
	codespacesInternalPortName = "CodespacesInternal"
)

// newPortsCmd returns a Cobra "ports" command that displays a table of available ports,
// according to the specified flags.
func newPortsCmd(app *App) *cobra.Command {
	var (
		selector *CodespaceSelector
		exporter cmdutil.Exporter
	)

	portsCmd := &cobra.Command{
		Use:   "ports",
		Short: "List ports in a codespace",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ListPorts(cmd.Context(), selector, exporter)
		},
	}

	selector = AddCodespaceSelector(portsCmd, app.apiClient)

	cmdutil.AddJSONFlags(portsCmd, &exporter, portFields)

	portsCmd.AddCommand(newPortsForwardCmd(app, selector))
	portsCmd.AddCommand(newPortsVisibilityCmd(app, selector))

	return portsCmd
}

// ListPorts lists known ports in a codespace.
func (a *App) ListPorts(ctx context.Context, selector *CodespaceSelector, exporter cmdutil.Exporter) (err error) {
	codespace, err := selector.Select(ctx)
	if err != nil {
		return err
	}

	devContainerCh := getDevContainer(ctx, a.apiClient, codespace)

	session, err := startLiveShareSession(ctx, codespace, a, false, "")
	if err != nil {
		return err
	}
	defer safeClose(session, &err)

	var ports []*liveshare.Port
	err = a.RunWithProgress("Fetching ports", func() (err error) {
		ports, err = session.GetSharedServers(ctx)
		return
	})
	if err != nil {
		return fmt.Errorf("error getting ports of shared servers: %w", err)
	}

	devContainerResult := <-devContainerCh
	if devContainerResult.err != nil {
		// Warn about failure to read the devcontainer file. Not a codespace command error.
		a.errLogger.Printf("Failed to get port names: %v", devContainerResult.err.Error())
	}

	var portInfos []*portInfo

	for _, p := range ports {
		// filter out internal ports from list
		if strings.HasPrefix(p.SessionName, vscodeServerPortName) || strings.HasPrefix(p.SessionName, codespacesInternalPortName) {
			continue
		}
		portInfos = append(portInfos, &portInfo{
			Port:         p,
			codespace:    codespace,
			devContainer: devContainerResult.devContainer,
		})
	}

	if err := a.io.StartPager(); err != nil {
		a.errLogger.Printf("error starting pager: %v", err)
	}
	defer a.io.StopPager()

	if exporter != nil {
		return exporter.Write(a.io, portInfos)
	}

	cs := a.io.ColorScheme()
	//nolint:staticcheck // SA1019: utils.NewTablePrinter is deprecated: use internal/tableprinter
	tp := utils.NewTablePrinter(a.io)

	if tp.IsTTY() {
		tp.AddField("LABEL", nil, nil)
		tp.AddField("PORT", nil, nil)
		tp.AddField("VISIBILITY", nil, nil)
		tp.AddField("BROWSE URL", nil, nil)
		tp.EndRow()
	}

	for _, port := range portInfos {
		tp.AddField(port.Label(), nil, nil)
		tp.AddField(strconv.Itoa(port.SourcePort), nil, cs.Yellow)
		tp.AddField(port.Privacy, nil, nil)
		tp.AddField(port.BrowseURL(), nil, nil)
		tp.EndRow()
	}
	return tp.Render()
}

type portInfo struct {
	*liveshare.Port
	codespace    *api.Codespace
	devContainer *devContainer
}

func (pi *portInfo) BrowseURL() string {
	return fmt.Sprintf("https://%s-%d.preview.app.github.dev", pi.codespace.Name, pi.Port.SourcePort)
}

func (pi *portInfo) Label() string {
	if pi.devContainer != nil {
		portStr := strconv.Itoa(pi.Port.SourcePort)
		if attributes, ok := pi.devContainer.PortAttributes[portStr]; ok {
			return attributes.Label
		}
	}
	return ""
}

var portFields = []string{
	"sourcePort",
	// "destinationPort", // TODO(mislav): this appears to always be blank?
	"visibility",
	"label",
	"browseUrl",
}

func (pi *portInfo) ExportData(fields []string) map[string]interface{} {
	data := map[string]interface{}{}

	for _, f := range fields {
		switch f {
		case "sourcePort":
			data[f] = pi.Port.SourcePort
		case "destinationPort":
			data[f] = pi.Port.DestinationPort
		case "visibility":
			data[f] = pi.Port.Privacy
		case "label":
			data[f] = pi.Label()
		case "browseUrl":
			data[f] = pi.BrowseURL()
		default:
			panic("unknown field: " + f)
		}
	}

	return data
}

type devContainerResult struct {
	devContainer *devContainer
	err          error
}

type devContainer struct {
	PortAttributes map[string]portAttribute `json:"portsAttributes"`
}

type portAttribute struct {
	Label string `json:"label"`
}

func getDevContainer(ctx context.Context, apiClient apiClient, codespace *api.Codespace) <-chan devContainerResult {
	ch := make(chan devContainerResult, 1)
	go func() {
		contents, err := apiClient.GetCodespaceRepositoryContents(ctx, codespace, ".devcontainer/devcontainer.json")
		if err != nil {
			ch <- devContainerResult{nil, fmt.Errorf("error getting content: %w", err)}
			return
		}

		if contents == nil {
			ch <- devContainerResult{nil, nil}
			return
		}

		convertedJSON := normalizeJSON(jsonc.ToJSON(contents))
		if !jsonc.Valid(convertedJSON) {
			ch <- devContainerResult{nil, errors.New("failed to convert json to standard json")}
			return
		}

		var container devContainer
		if err := json.Unmarshal(convertedJSON, &container); err != nil {
			ch <- devContainerResult{nil, fmt.Errorf("error unmarshaling: %w", err)}
			return
		}

		ch <- devContainerResult{&container, nil}
	}()
	return ch
}

func newPortsVisibilityCmd(app *App, selector *CodespaceSelector) *cobra.Command {
	return &cobra.Command{
		Use:     "visibility <port>:{public|private|org}...",
		Short:   "Change the visibility of the forwarded port",
		Example: "gh codespace ports visibility 80:org 3000:private 8000:public",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.UpdatePortVisibility(cmd.Context(), selector, args)
		},
	}
}

type ErrUpdatingPortVisibility struct {
	port       int
	visibility string
	err        error
}

func newErrUpdatingPortVisibility(port int, visibility string, err error) *ErrUpdatingPortVisibility {
	return &ErrUpdatingPortVisibility{
		port:       port,
		visibility: visibility,
		err:        err,
	}
}

func (e *ErrUpdatingPortVisibility) Error() string {
	return fmt.Sprintf("error waiting for port %d to update to %s: %s", e.port, e.visibility, e.err)
}

func (e *ErrUpdatingPortVisibility) Unwrap() error {
	return e.err
}

var errUpdatePortVisibilityForbidden = errors.New("organization admin has forbidden this privacy setting")

func (a *App) UpdatePortVisibility(ctx context.Context, selector *CodespaceSelector, args []string) (err error) {
	ports, err := a.parsePortVisibilities(args)
	if err != nil {
		return fmt.Errorf("error parsing port arguments: %w", err)
	}

	codespace, err := selector.Select(ctx)
	if err != nil {
		return err
	}

	session, err := codespaces.ConnectToLiveshare(ctx, a, noopLogger(), a.apiClient, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to codespace: %w", err)
	}
	defer safeClose(session, &err)

	// TODO: check if port visibility can be updated in parallel instead of sequentially
	for _, port := range ports {
		err := a.RunWithProgress(fmt.Sprintf("Updating port %d visibility to: %s", port.number, port.visibility), func() (err error) {
			// wait for success or failure
			g, ctx := errgroup.WithContext(ctx)
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			g.Go(func() error {
				updateNotif, err := session.WaitForPortNotification(ctx, port.number, liveshare.PortChangeKindUpdate)
				if err != nil {
					return fmt.Errorf("error waiting for port %d to update: %w", port.number, err)

				}
				if !updateNotif.Success {
					if updateNotif.StatusCode == http.StatusForbidden {
						return newErrUpdatingPortVisibility(port.number, port.visibility, errUpdatePortVisibilityForbidden)
					}
					return newErrUpdatingPortVisibility(port.number, port.visibility, errors.New(updateNotif.ErrorDetail))

				}
				return nil // success
			})

			g.Go(func() error {
				err := session.UpdateSharedServerPrivacy(ctx, port.number, port.visibility)
				if err != nil {
					return fmt.Errorf("error updating port %d to %s: %w", port.number, port.visibility, err)
				}
				return nil
			})

			// wait for success or failure
			err = g.Wait()
			return
		})
		if err != nil {
			return err
		}

	}

	return nil
}

type portVisibility struct {
	number     int
	visibility string
}

func (a *App) parsePortVisibilities(args []string) ([]portVisibility, error) {
	ports := make([]portVisibility, 0, len(args))
	for _, a := range args {
		fields := strings.Split(a, ":")
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid port visibility format for %q", a)
		}
		portStr, visibility := fields[0], fields[1]
		portNumber, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port number: %w", err)
		}
		ports = append(ports, portVisibility{portNumber, visibility})
	}
	return ports, nil
}

// NewPortsForwardCmd returns a Cobra "ports forward" subcommand, which forwards a set of
// port pairs from the codespace to localhost.
func newPortsForwardCmd(app *App, selector *CodespaceSelector) *cobra.Command {
	return &cobra.Command{
		Use:   "forward <remote-port>:<local-port>...",
		Short: "Forward ports",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ForwardPorts(cmd.Context(), selector, args)
		},
	}
}

func (a *App) ForwardPorts(ctx context.Context, selector *CodespaceSelector, ports []string) (err error) {
	portPairs, err := getPortPairs(ports)
	if err != nil {
		return fmt.Errorf("get port pairs: %w", err)
	}

	codespace, err := selector.Select(ctx)
	if err != nil {
		return err
	}

	session, err := codespaces.ConnectToLiveshare(ctx, a, noopLogger(), a.apiClient, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to codespace: %w", err)
	}
	defer safeClose(session, &err)

	// Run forwarding of all ports concurrently, aborting all of
	// them at the first failure, including cancellation of the context.
	group, ctx := errgroup.WithContext(ctx)
	for _, pair := range portPairs {
		pair := pair
		group.Go(func() error {
			listen, _, err := codespaces.ListenTCP(pair.local)
			if err != nil {
				return err
			}
			defer listen.Close()

			a.errLogger.Printf("Forwarding ports: remote %d <=> local %d", pair.remote, pair.local)
			name := fmt.Sprintf("share-%d", pair.remote)
			fwd := liveshare.NewPortForwarder(session, name, pair.remote, false)
			return fwd.ForwardToListener(ctx, listen) // error always non-nil
		})
	}
	return group.Wait() // first error
}

type portPair struct {
	remote, local int
}

// getPortPairs parses a list of strings of form "%d:%d" into pairs of (remote, local) numbers.
func getPortPairs(ports []string) ([]portPair, error) {
	pp := make([]portPair, 0, len(ports))

	for _, portString := range ports {
		parts := strings.Split(portString, ":")
		if len(parts) < 2 {
			return nil, fmt.Errorf("port pair: %q is not valid", portString)
		}

		remote, err := strconv.Atoi(parts[0])
		if err != nil {
			return pp, fmt.Errorf("convert remote port to int: %w", err)
		}

		local, err := strconv.Atoi(parts[1])
		if err != nil {
			return pp, fmt.Errorf("convert local port to int: %w", err)
		}

		pp = append(pp, portPair{remote, local})
	}

	return pp, nil
}

func normalizeJSON(j []byte) []byte {
	// remove trailing commas
	return bytes.ReplaceAll(j, []byte("},}"), []byte("}}"))
}
