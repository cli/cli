package codespaces

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/github/ghcs/api"
	"github.com/github/go-liveshare"
)

var (
	ErrNoCodespaces = errors.New("You have no codespaces.")
)

func ChooseCodespace(ctx context.Context, apiClient *api.API, user *api.User) (*api.Codespace, error) {
	codespaces, err := apiClient.ListCodespaces(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("error getting codespaces: %v", err)
	}

	if len(codespaces) == 0 {
		return nil, ErrNoCodespaces
	}

	codespaces.SortByCreatedAt()

	codespacesByName := make(map[string]*api.Codespace)
	codespacesNames := make([]string, 0, len(codespaces))
	for _, codespace := range codespaces {
		codespacesByName[codespace.Name] = codespace
		codespacesNames = append(codespacesNames, codespace.Name)
	}

	sshSurvey := []*survey.Question{
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
	if err := survey.Ask(sshSurvey, &answers); err != nil {
		return nil, fmt.Errorf("error getting answers: %v", err)
	}

	codespace := codespacesByName[answers.Codespace]
	return codespace, nil
}

type logger interface {
	Print(v ...interface{}) (int, error)
	Println(v ...interface{}) (int, error)
}

func ConnectToLiveshare(ctx context.Context, log logger, apiClient *api.API, token string, codespace *api.Codespace) (client *liveshare.Client, err error) {
	if codespace.Environment.State != api.CodespaceEnvironmentStateAvailable {
		log.Println("Starting your codespace...")
		if err := apiClient.StartCodespace(ctx, token, codespace); err != nil {
			return nil, fmt.Errorf("error starting codespace: %v", err)
		}
	}

	retries := 0
	for codespace.Environment.Connection.SessionID == "" || codespace.Environment.State != api.CodespaceEnvironmentStateAvailable {
		if retries > 1 {
			if retries%2 == 0 {
				log.Print(".")
			}

			time.Sleep(1 * time.Second)
		}

		if retries == 30 {
			return nil, errors.New("timed out while waiting for the codespace to start")
		}

		codespace, err = apiClient.GetCodespace(ctx, token, codespace.OwnerLogin, codespace.Name)
		if err != nil {
			return nil, fmt.Errorf("error getting codespace: %v", err)
		}

		retries += 1
	}

	if retries >= 2 {
		log.Print("\n")
	}

	log.Println("Connecting to your codespace...")

	lsclient, err := liveshare.NewClient(
		liveshare.WithConnection(liveshare.Connection{
			SessionID:     codespace.Environment.Connection.SessionID,
			SessionToken:  codespace.Environment.Connection.SessionToken,
			RelaySAS:      codespace.Environment.Connection.RelaySAS,
			RelayEndpoint: codespace.Environment.Connection.RelayEndpoint,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating live share: %v", err)
	}

	if err := lsclient.Join(ctx); err != nil {
		return nil, fmt.Errorf("error joining liveshare client: %v", err)
	}

	return lsclient, nil
}

// StartSSHServer starts and installs the SSH server in the codespace
// returns the remote port where it is running, the user to use to login
// or an error if something failed.
func StartSSHServer(ctx context.Context, client *liveshare.Client) (serverPort int, user string, err error) {
	sshServer, err := liveshare.NewSSHServer(client)
	if err != nil {
		return 0, "", fmt.Errorf("error creating live share: %v", err)
	}

	sshServerStartResult, err := sshServer.StartRemoteServer(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("error starting live share: %v", err)
	}

	if !sshServerStartResult.Result {
		return 0, "", errors.New(sshServerStartResult.Message)
	}

	portInt, err := strconv.Atoi(sshServerStartResult.ServerPort)
	if err != nil {
		return 0, "", fmt.Errorf("error parsing port: %v", err)
	}

	return portInt, sshServerStartResult.User, nil
}

func GetOrChooseCodespace(ctx context.Context, apiClient *api.API, user *api.User, codespaceName string) (codespace *api.Codespace, token string, err error) {
	if codespaceName == "" {
		codespace, err = ChooseCodespace(ctx, apiClient, user)
		if err != nil {
			if err == ErrNoCodespaces {
				return nil, "", err
			}
			return nil, "", fmt.Errorf("choosing codespace: %v", err)
		}
		codespaceName = codespace.Name

		token, err = apiClient.GetCodespaceToken(ctx, user.Login, codespaceName)
		if err != nil {
			return nil, "", fmt.Errorf("getting codespace token: %v", err)
		}
	} else {
		token, err = apiClient.GetCodespaceToken(ctx, user.Login, codespaceName)
		if err != nil {
			return nil, "", fmt.Errorf("getting codespace token for given codespace: %v", err)
		}

		codespace, err = apiClient.GetCodespace(ctx, token, user.Login, codespaceName)
		if err != nil {
			return nil, "", fmt.Errorf("getting full codespace details: %v", err)
		}
	}

	return codespace, token, nil
}
