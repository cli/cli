package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/github/ghcs/api"
	"github.com/github/go-liveshare"
	"github.com/muhammadmuzzammil1998/jsonc"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func NewPortsCmd() *cobra.Command {
	portsCmd := &cobra.Command{
		Use:   "ports",
		Short: "ports",
		Long:  "ports",
		RunE: func(cmd *cobra.Command, args []string) error {
			return Ports()
		},
	}

	return portsCmd
}

func init() {
	rootCmd.AddCommand(NewPortsCmd())
}

func Ports() error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	codespaces, err := apiClient.ListCodespaces(ctx, user)
	if err != nil {
		return fmt.Errorf("error getting codespaces: %v", err)
	}

	if len(codespaces) == 0 {
		fmt.Println("You have no codespaces.")
		return nil
	}

	codespaces.SortByCreatedAt()

	codespacesByName := make(map[string]*api.Codespace)
	codespacesNames := make([]string, 0, len(codespaces))
	for _, codespace := range codespaces {
		codespacesByName[codespace.Name] = codespace
		codespacesNames = append(codespacesNames, codespace.Name)
	}

	portsSurvey := []*survey.Question{
		{
			Name: "codespace",
			Prompt: &survey.Select{
				Message: "Choose Codespace:",
				Options: codespacesNames,
				Default: codespacesNames[0],
			},
			Validate: survey.Required,
		},
	}

	answers := struct {
		Codespace string
	}{}
	if err := survey.Ask(portsSurvey, &answers); err != nil {
		return fmt.Errorf("error getting answers: %v", err)
	}

	codespace := codespacesByName[answers.Codespace]
	devContainerCh := getDevContainer(ctx, apiClient, codespace)

	token, err := apiClient.GetCodespaceToken(ctx, codespace)
	if err != nil {
		return fmt.Errorf("error getting codespace token: %v", err)
	}

	if codespace.Environment.State != api.CodespaceEnvironmentStateAvailable {
		fmt.Println("Starting your codespace...")
		if err := apiClient.StartCodespace(ctx, token, codespace); err != nil {
			return fmt.Errorf("error starting codespace: %v", err)
		}
	}

	retries := 0
	for codespace.Environment.Connection.SessionID == "" || codespace.Environment.State != api.CodespaceEnvironmentStateAvailable {
		if retries > 1 {
			if retries%2 == 0 {
				fmt.Print(".")
			}

			time.Sleep(1 * time.Second)
		}

		if retries == 30 {
			return errors.New("timed out while waiting for the codespace to start")
		}

		codespace, err = apiClient.GetCodespace(ctx, token, codespace.OwnerLogin, codespace.Name)
		if err != nil {
			return fmt.Errorf("error getting codespace: %v", err)
		}

		retries += 1
	}

	if retries >= 2 {
		fmt.Print("\n")
	}

	fmt.Println("Connecting to your codespace...")

	liveShare, err := liveshare.New(
		liveshare.WithWorkspaceID(codespace.Environment.Connection.SessionID),
		liveshare.WithToken(codespace.Environment.Connection.SessionToken),
	)
	if err != nil {
		return fmt.Errorf("error creating live share: %v", err)
	}

	liveShareClient := liveShare.NewClient()
	if err := liveShareClient.Join(ctx); err != nil {
		return fmt.Errorf("error joining liveshare client: %v", err)
	}

	fmt.Println("Loading ports...")
	ports, err := getPorts(ctx, liveShareClient)
	if err != nil {
		return fmt.Errorf("error getting ports: %v", err)
	}

	devContainerResult := <-devContainerCh
	if devContainerResult.Err != nil {
		fmt.Println("Failed to get port names: %v", devContainerResult.Err.Error())
	}

	table := tablewriter.NewWriter(os.Stdout)
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

func getPorts(ctx context.Context, liveShareClient *liveshare.Client) (liveshare.Ports, error) {
	server, err := liveShareClient.NewServer()
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

		convertedJSON := jsonc.ToJSON(contents)
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
