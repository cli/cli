package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/github/ghcs/api"
	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/codespaces"
	"github.com/github/go-liveshare"
	"github.com/muhammadmuzzammil1998/jsonc"
	"github.com/spf13/cobra"
)

type PortsOptions struct {
	CodespaceName string
	AsJSON        bool
}

func NewPortsCmd() *cobra.Command {
	opts := &PortsOptions{}

	portsCmd := &cobra.Command{
		Use:   "ports",
		Short: "Forward ports from a GitHub Codespace.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return Ports(opts)
		},
	}

	portsCmd.Flags().StringVarP(&opts.CodespaceName, "codespace", "c", "", "The `name` of the Codespace to use")
	portsCmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")

	portsCmd.AddCommand(NewPortsPublicCmd())
	portsCmd.AddCommand(NewPortsPrivateCmd())
	portsCmd.AddCommand(NewPortsForwardCmd())

	return portsCmd
}

func init() {
	rootCmd.AddCommand(NewPortsCmd())
}

func Ports(opts *PortsOptions) error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()
	log := output.NewLogger(os.Stdout, os.Stderr, opts.AsJSON)

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	codespace, token, err := codespaces.GetOrChooseCodespace(ctx, apiClient, user, opts.CodespaceName)
	if err != nil {
		if err == codespaces.ErrNoCodespaces {
			return err
		}
		return fmt.Errorf("error choosing codespace: %v", err)
	}

	devContainerCh := getDevContainer(ctx, apiClient, codespace)

	liveShareClient, err := codespaces.ConnectToLiveshare(ctx, log, apiClient, token, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to liveshare: %v", err)
	}

	log.Println("Loading ports...")
	ports, err := getPorts(ctx, liveShareClient)
	if err != nil {
		return fmt.Errorf("error getting ports: %v", err)
	}

	devContainerResult := <-devContainerCh
	if devContainerResult.Err != nil {
		_, _ = log.Errorf("Failed to get port names: %v\n", devContainerResult.Err.Error())
	}

	table := output.NewTable(os.Stdout, opts.AsJSON)
	table.SetHeader([]string{"Label", "Source Port", "Destination Port", "Public", "Browse URL"})
	for _, port := range ports {
		sourcePort := strconv.Itoa(port.SourcePort)
		var portName string
		if devContainerResult.DevContainer != nil {
			if attributes, ok := devContainerResult.DevContainer.PortAttributes[sourcePort]; ok {
				portName = attributes.Label
			}
		}

		table.Append([]string{
			portName,
			sourcePort,
			strconv.Itoa(port.DestinationPort),
			strings.ToUpper(strconv.FormatBool(port.IsPublic)),
			fmt.Sprintf("https://%s-%s.githubpreview.dev/", codespace.Name, sourcePort),
		})
	}
	table.Render()

	return nil
}

func getPorts(ctx context.Context, lsclient *liveshare.Client) (liveshare.Ports, error) {
	server, err := liveshare.NewServer(lsclient)
	if err != nil {
		return nil, fmt.Errorf("error creating server: %v", err)
	}

	ports, err := server.GetSharedServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting shared servers: %v", err)
	}

	return ports, nil
}

type devContainerResult struct {
	DevContainer *devContainer
	Err          error
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

func NewPortsPublicCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "public",
		Short: "public",
		Long:  "public",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return errors.New("[codespace_name] [source] port number are required.")
			}

			log := output.NewLogger(os.Stdout, os.Stderr, false)
			return updatePortVisibility(log, args[0], args[1], true)
		},
	}
}

func NewPortsPrivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "private",
		Short: "private",
		Long:  "private",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return errors.New("[codespace_name] [source] port number are required.")
			}

			log := output.NewLogger(os.Stdout, os.Stderr, false)
			return updatePortVisibility(log, args[0], args[1], false)
		},
	}
}

func updatePortVisibility(log *output.Logger, codespaceName, sourcePort string, public bool) error {
	ctx := context.Background()
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	token, err := apiClient.GetCodespaceToken(ctx, user.Login, codespaceName)
	if err != nil {
		return fmt.Errorf("error getting codespace token: %v", err)
	}

	codespace, err := apiClient.GetCodespace(ctx, token, user.Login, codespaceName)
	if err != nil {
		return fmt.Errorf("error getting codespace: %v", err)
	}

	lsclient, err := codespaces.ConnectToLiveshare(ctx, log, apiClient, token, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to liveshare: %v", err)
	}

	server, err := liveshare.NewServer(lsclient)
	if err != nil {
		return fmt.Errorf("error creating server: %v", err)
	}

	port, err := strconv.Atoi(sourcePort)
	if err != nil {
		return fmt.Errorf("error reading port number: %v", err)
	}

	if err := server.UpdateSharedVisibility(ctx, port, public); err != nil {
		return fmt.Errorf("error update port to public: %v", err)
	}

	state := "PUBLIC"
	if !public {
		state = "PRIVATE"
	}
	log.Printf("Port %s is now %s.\n", sourcePort, state)

	return nil
}

func NewPortsForwardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "forward",
		Short: "forward",
		Long:  "forward",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 3 {
				return errors.New("[codespace_name] [source] [dst] port number are required.")
			}

			log := output.NewLogger(os.Stdout, os.Stderr, false)
			return forwardPort(log, args[0], args[1], args[2])
		},
	}
}

func forwardPort(log *output.Logger, codespaceName, sourcePort, destPort string) error {
	ctx := context.Background()
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	token, err := apiClient.GetCodespaceToken(ctx, user.Login, codespaceName)
	if err != nil {
		return fmt.Errorf("error getting codespace token: %v", err)
	}

	codespace, err := apiClient.GetCodespace(ctx, token, user.Login, codespaceName)
	if err != nil {
		return fmt.Errorf("error getting codespace: %v", err)
	}

	lsclient, err := codespaces.ConnectToLiveshare(ctx, log, apiClient, token, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to liveshare: %v", err)
	}

	server, err := liveshare.NewServer(lsclient)
	if err != nil {
		return fmt.Errorf("error creating server: %v", err)
	}

	sourcePortInt, err := strconv.Atoi(sourcePort)
	if err != nil {
		return fmt.Errorf("error reading source port: %v", err)
	}

	dstPortInt, err := strconv.Atoi(destPort)
	if err != nil {
		return fmt.Errorf("error reading destination port: %v", err)
	}

	if err := server.StartSharing(ctx, "share-"+sourcePort, sourcePortInt); err != nil {
		return fmt.Errorf("error sharing source port: %v", err)
	}

	log.Println("Forwarding port: " + sourcePort + " -> " + destPort)
	portForwarder := liveshare.NewPortForwarder(lsclient, server, dstPortInt)
	if err := portForwarder.Start(ctx); err != nil {
		return fmt.Errorf("error forwarding port: %v", err)
	}

	return nil
}

func normalizeJSON(j []byte) []byte {
	// remove trailing commas
	return bytes.ReplaceAll(j, []byte("},}"), []byte("}}"))
}
