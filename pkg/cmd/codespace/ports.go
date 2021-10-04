package codespace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmd/codespace/output"
	"github.com/cli/cli/v2/pkg/liveshare"
	"github.com/muhammadmuzzammil1998/jsonc"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// newPortsCmd returns a Cobra "ports" command that displays a table of available ports,
// according to the specified flags.
func newPortsCmd(app *App) *cobra.Command {
	var (
		codespace string
		asJSON    bool
	)

	portsCmd := &cobra.Command{
		Use:   "ports",
		Short: "List ports in a codespace",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ListPorts(cmd.Context(), codespace, asJSON)
		},
	}

	portsCmd.PersistentFlags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
	portsCmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	portsCmd.AddCommand(newPortsPublicCmd(app))
	portsCmd.AddCommand(newPortsPrivateCmd(app))
	portsCmd.AddCommand(newPortsForwardCmd(app))

	return portsCmd
}

// ListPorts lists known ports in a codespace.
func (a *App) ListPorts(ctx context.Context, codespaceName string, asJSON bool) (err error) {
	user, err := a.apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %w", err)
	}

	codespace, token, err := getOrChooseCodespace(ctx, a.apiClient, user, codespaceName)
	if err != nil {
		// TODO(josebalius): remove special handling of this error here and it other places
		if err == errNoCodespaces {
			return err
		}
		return fmt.Errorf("error choosing codespace: %w", err)
	}

	devContainerCh := getDevContainer(ctx, a.apiClient, codespace)

	session, err := codespaces.ConnectToLiveshare(ctx, a.logger, a.apiClient, user.Login, token, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to Live Share: %w", err)
	}
	defer safeClose(session, &err)

	a.logger.Println("Loading ports...")
	ports, err := session.GetSharedServers(ctx)
	if err != nil {
		return fmt.Errorf("error getting ports of shared servers: %w", err)
	}

	devContainerResult := <-devContainerCh
	if devContainerResult.err != nil {
		// Warn about failure to read the devcontainer file. Not a codespace command error.
		_, _ = a.logger.Errorf("Failed to get port names: %v\n", devContainerResult.err.Error())
	}

	table := output.NewTable(os.Stdout, asJSON)
	table.SetHeader([]string{"Label", "Port", "Public", "Browse URL"})
	for _, port := range ports {
		sourcePort := strconv.Itoa(port.SourcePort)
		var portName string
		if devContainerResult.devContainer != nil {
			if attributes, ok := devContainerResult.devContainer.PortAttributes[sourcePort]; ok {
				portName = attributes.Label
			}
		}

		table.Append([]string{
			portName,
			sourcePort,
			strings.ToUpper(strconv.FormatBool(port.IsPublic)),
			fmt.Sprintf("https://%s-%s.githubpreview.dev/", codespace.Name, sourcePort),
		})
	}
	table.Render()

	return nil
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

// newPortsPublicCmd returns a Cobra "ports public" subcommand, which makes a given port public.
func newPortsPublicCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "public <port>",
		Short: "Mark port as public",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			codespace, err := cmd.Flags().GetString("codespace")
			if err != nil {
				// should only happen if flag is not defined
				// or if the flag is not of string type
				// since it's a persistent flag that we control it should never happen
				return fmt.Errorf("get codespace flag: %w", err)
			}

			return app.UpdatePortVisibility(cmd.Context(), codespace, args[0], true)
		},
	}
}

// newPortsPrivateCmd returns a Cobra "ports private" subcommand, which makes a given port private.
func newPortsPrivateCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "private <port>",
		Short: "Mark port as private",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			codespace, err := cmd.Flags().GetString("codespace")
			if err != nil {
				// should only happen if flag is not defined
				// or if the flag is not of string type
				// since it's a persistent flag that we control it should never happen
				return fmt.Errorf("get codespace flag: %w", err)
			}

			return app.UpdatePortVisibility(cmd.Context(), codespace, args[0], false)
		},
	}
}

func (a *App) UpdatePortVisibility(ctx context.Context, codespaceName, sourcePort string, public bool) (err error) {
	user, err := a.apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %w", err)
	}

	codespace, token, err := getOrChooseCodespace(ctx, a.apiClient, user, codespaceName)
	if err != nil {
		if err == errNoCodespaces {
			return err
		}
		return fmt.Errorf("error getting codespace: %w", err)
	}

	session, err := codespaces.ConnectToLiveshare(ctx, a.logger, a.apiClient, user.Login, token, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to Live Share: %w", err)
	}
	defer safeClose(session, &err)

	port, err := strconv.Atoi(sourcePort)
	if err != nil {
		return fmt.Errorf("error reading port number: %w", err)
	}

	if err := session.UpdateSharedVisibility(ctx, port, public); err != nil {
		return fmt.Errorf("error update port to public: %w", err)
	}

	state := "PUBLIC"
	if !public {
		state = "PRIVATE"
	}
	a.logger.Printf("Port %s is now %s.\n", sourcePort, state)

	return nil
}

// NewPortsForwardCmd returns a Cobra "ports forward" subcommand, which forwards a set of
// port pairs from the codespace to localhost.
func newPortsForwardCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "forward <remote-port>:<local-port>...",
		Short: "Forward ports",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			codespace, err := cmd.Flags().GetString("codespace")
			if err != nil {
				// should only happen if flag is not defined
				// or if the flag is not of string type
				// since it's a persistent flag that we control it should never happen
				return fmt.Errorf("get codespace flag: %w", err)
			}

			return app.ForwardPorts(cmd.Context(), codespace, args)
		},
	}
}

func (a *App) ForwardPorts(ctx context.Context, codespaceName string, ports []string) (err error) {
	portPairs, err := getPortPairs(ports)
	if err != nil {
		return fmt.Errorf("get port pairs: %w", err)
	}

	user, err := a.apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %w", err)
	}

	codespace, token, err := getOrChooseCodespace(ctx, a.apiClient, user, codespaceName)
	if err != nil {
		if err == errNoCodespaces {
			return err
		}
		return fmt.Errorf("error getting codespace: %w", err)
	}

	session, err := codespaces.ConnectToLiveshare(ctx, a.logger, a.apiClient, user.Login, token, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to Live Share: %w", err)
	}
	defer safeClose(session, &err)

	// Run forwarding of all ports concurrently, aborting all of
	// them at the first failure, including cancellation of the context.
	group, ctx := errgroup.WithContext(ctx)
	for _, pair := range portPairs {
		pair := pair
		group.Go(func() error {
			listen, err := net.Listen("tcp", fmt.Sprintf(":%d", pair.local))
			if err != nil {
				return err
			}
			defer listen.Close()
			a.logger.Printf("Forwarding ports: remote %d <=> local %d\n", pair.remote, pair.local)
			name := fmt.Sprintf("share-%d", pair.remote)
			fwd := liveshare.NewPortForwarder(session, name, pair.remote)
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
