package codespaces

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/github/ghcs/api"
)

type PostCreateStateStatus string

func (p PostCreateStateStatus) String() string {
	return strings.Title(string(p))
}

const (
	PostCreateStateRunning PostCreateStateStatus = "running"
	PostCreateStateSuccess PostCreateStateStatus = "succeeded"
	PostCreateStateFailed  PostCreateStateStatus = "failed"
)

type PostCreateStatesResult struct {
	PostCreateStates []PostCreateState
	Err              error
}

type PostCreateState struct {
	Name   string                `json:"name"`
	Status PostCreateStateStatus `json:"status"`
}

func PollPostCreateStates(ctx context.Context, log logger, apiClient *api.API, user *api.User, codespace *api.Codespace, poller func([]PostCreateState)) error {
	token, err := apiClient.GetCodespaceToken(ctx, user.Login, codespace.Name)
	if err != nil {
		return fmt.Errorf("getting codespace token: %v", err)
	}

	lsclient, err := ConnectToLiveshare(ctx, log, apiClient, user.Login, token, codespace)
	if err != nil {
		return fmt.Errorf("connect to liveshare: %v", err)
	}

	tunnelPort, connClosed, err := MakeSSHTunnel(ctx, lsclient, 0)
	if err != nil {
		return fmt.Errorf("make ssh tunnel: %v", err)
	}

	t := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-connClosed:
			return fmt.Errorf("connection closed: %v", err)
		case <-t.C:
			states, err := getPostCreateOutput(ctx, tunnelPort, codespace)
			if err != nil {
				return fmt.Errorf("get post create output: %v", err)
			}

			poller(states)
		}
	}

	return nil
}

func getPostCreateOutput(ctx context.Context, tunnelPort int, codespace *api.Codespace) ([]PostCreateState, error) {
	stdout, err := RunCommand(
		ctx, tunnelPort, sshDestination(codespace),
		"cat /workspaces/.codespaces/shared/postCreateOutput.json",
	)
	if err != nil {
		return nil, fmt.Errorf("run command: %v", err)
	}
	defer stdout.Close()

	b, err := ioutil.ReadAll(stdout)
	if err != nil {
		return nil, fmt.Errorf("read output: %v", err)
	}

	var output struct {
		Steps []PostCreateState `json:"steps"`
	}
	if err := json.Unmarshal(b, &output); err != nil {
		return nil, fmt.Errorf("unmarshal output: %v", err)
	}

	return output.Steps, nil
}

func sshDestination(codespace *api.Codespace) string {
	user := "codespace"
	if codespace.RepositoryNWO == "github/github" {
		user = "root"
	}
	return user + "@localhost"
}
