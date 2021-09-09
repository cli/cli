package main

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

	"github.com/github/ghcs/api"
	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/codespaces"
	"github.com/github/go-liveshare"
	"github.com/muhammadmuzzammil1998/jsonc"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// newPortsCmd returns a Cobra "ports" command that displays a table of available ports,
// according to the specified flags.
func newPortsCmd() *cobra.Command {
	var (
		codespace string
		asJSON    bool
	)

	portsCmd := &cobra.Command{
		Use:   "ports",
		Short: "List ports in a codespace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ports(codespace, asJSON)
		},
	}

	portsCmd.PersistentFlags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
	portsCmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	portsCmd.AddCommand(newPortsPublicCmd())
	portsCmd.AddCommand(newPortsPrivateCmd())
	portsCmd.AddCommand(newPortsForwardCmd())

	return portsCmd
}

func init() {
	rootCmd.AddCommand(newPortsCmd())
}

func ports(codespaceName string, asJSON bool) error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()
	log := output.NewLogger(os.Stdout, os.Stderr, asJSON)

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	codespace, token, err := codespaces.GetOrChooseCodespace(ctx, apiClient, user, codespaceName)
	if err != nil {
		if err == codespaces.ErrNoCodespaces {
			return err
		}
		return fmt.Errorf("error choosing codespace: %v", err)
	}

	devContainerCh := getDevContainer(ctx, apiClient, codespace)

	session, err := codespaces.ConnectToLiveshare(ctx, log, apiClient, user.Login, token, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to Live Share: %v", err)
	}

	log.Println("Loading ports...")
	ports, err := session.GetSharedServers(ctx)
	if err != nil {
		return fmt.Errorf("error getting ports of shared servers: %v", err)
	}

	devContainerResult := <-devContainerCh
	if devContainerResult.err != nil {
		// Warn about failure to read the devcontainer file. Not a ghcs command error.
		_, _ = log.Errorf("Failed to get port names: %v\n", devContainerResult.err.Error())
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

func getDevContainer(ctx context.Context, apiClient *api.API, codespace *api.Codespace) <-chan devContainerResult {
	ch := make(chan devContainerResult)
	go func() {
		contents, err := apiClient.GetCodespaceRepositoryContents(ctx, codespace, ".devcontainer/devcontainer.json")
		if err != nil {
			ch <- devContainerResult{nil, fmt.Errorf("error getting content: %v", err)}
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
			ch <- devContainerResult{nil, fmt.Errorf("error unmarshaling: %v", err)}
			return
		}

		ch <- devContainerResult{&container, nil}
	}()
	return ch
}

// newPortsPublicCmd returns a Cobra "ports public" subcommand, which makes a given port public.
func newPortsPublicCmd() *cobra.Command {
	newPortsPublicCmd := &cobra.Command{
		Use:   "public <port>",
		Short: "Mark port as public",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			codespace, err := cmd.Flags().GetString("codespace")
			if err != nil {
				// should only happen if flag is not defined
				// or if the flag is not of string type
				// since it's a persistent flag that we control it should never happen
				return fmt.Errorf("get codespace flag: %v", err)
			}

			log := output.NewLogger(os.Stdout, os.Stderr, false)

			port := args[0]
			if len(args) > 1 {
				log.Errorln("<codespace> argument is deprecated. Use --codespace instead.")
				codespace, port = args[0], args[1]
			}

			return updatePortVisibility(log, codespace, port, true)
		},
	}

	return newPortsPublicCmd
}

// newPortsPrivateCmd returns a Cobra "ports private" subcommand, which makes a given port private.
func newPortsPrivateCmd() *cobra.Command {
	newPortsPrivateCmd := &cobra.Command{
		Use:   "private <port>",
		Short: "Mark port as private",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			codespace, err := cmd.Flags().GetString("codespace")
			if err != nil {
				// should only happen if flag is not defined
				// or if the flag is not of string type
				// since it's a persistent flag that we control it should never happen
				return fmt.Errorf("get codespace flag: %v", err)
			}

			log := output.NewLogger(os.Stdout, os.Stderr, false)

			port := args[0]
			if len(args) > 1 {
				log.Errorln("<codespace> argument is deprecated. Use --codespace instead.")
				codespace, port = args[0], args[1]
			}

			return updatePortVisibility(log, codespace, port, false)
		},
	}

	return newPortsPrivateCmd
}

func updatePortVisibility(log *output.Logger, codespaceName, sourcePort string, public bool) error {
	ctx := context.Background()
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	codespace, token, err := codespaces.GetOrChooseCodespace(ctx, apiClient, user, codespaceName)
	if err != nil {
		if err == codespaces.ErrNoCodespaces {
			return err
		}
		return fmt.Errorf("error getting codespace: %v", err)
	}

	session, err := codespaces.ConnectToLiveshare(ctx, log, apiClient, user.Login, token, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to Live Share: %v", err)
	}

	port, err := strconv.Atoi(sourcePort)
	if err != nil {
		return fmt.Errorf("error reading port number: %v", err)
	}

	if err := session.UpdateSharedVisibility(ctx, port, public); err != nil {
		return fmt.Errorf("error update port to public: %v", err)
	}

	state := "PUBLIC"
	if !public {
		state = "PRIVATE"
	}
	log.Printf("Port %s is now %s.\n", sourcePort, state)

	return nil
}

// NewPortsForwardCmd returns a Cobra "ports forward" subcommand, which forwards a set of
// port pairs from the codespace to localhost.
func newPortsForwardCmd() *cobra.Command {
	newPortsForwardCmd := &cobra.Command{
		Use:   "forward <remote-port>:<local-port>...",
		Short: "Forward ports",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			codespace, err := cmd.Flags().GetString("codespace")
			if err != nil {
				// should only happen if flag is not defined
				// or if the flag is not of string type
				// since it's a persistent flag that we control it should never happen
				return fmt.Errorf("get codespace flag: %v", err)
			}

			log := output.NewLogger(os.Stdout, os.Stderr, false)

			ports := args[0:]
			if len(args) > 1 && !strings.Contains(args[0], ":") {
				// assume this is a codespace name
				log.Errorln("<codespace> argument is deprecated. Use --codespace instead.")
				codespace = args[0]
				ports = args[1:]
			}

			return forwardPorts(log, codespace, ports)
		},
	}

	return newPortsForwardCmd
}

func forwardPorts(log *output.Logger, codespaceName string, ports []string) error {
	ctx := context.Background()
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))

	portPairs, err := getPortPairs(ports)
	if err != nil {
		return fmt.Errorf("get port pairs: %v", err)
	}

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	codespace, token, err := codespaces.GetOrChooseCodespace(ctx, apiClient, user, codespaceName)
	if err != nil {
		if err == codespaces.ErrNoCodespaces {
			return err
		}
		return fmt.Errorf("error getting codespace: %v", err)
	}

	session, err := codespaces.ConnectToLiveshare(ctx, log, apiClient, user.Login, token, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to Live Share: %v", err)
	}

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
			log.Printf("Forwarding ports: remote %d <=> local %d\n", pair.remote, pair.local)
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
			return pp, fmt.Errorf("convert remote port to int: %v", err)
		}

		local, err := strconv.Atoi(parts[1])
		if err != nil {
			return pp, fmt.Errorf("convert local port to int: %v", err)
		}

		pp = append(pp, portPair{remote, local})
	}

	return pp, nil
}

func normalizeJSON(j []byte) []byte {
	// remove trailing commas
	return bytes.ReplaceAll(j, []byte("},}"), []byte("}}"))
}
