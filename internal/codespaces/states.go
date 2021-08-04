package codespaces

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/github/ghcs/api"
)

type PostCreateStateStatus string

const (
	PostCreateStateRunning PostCreateStateStatus = "running"
	PostCreateStateSuccess PostCreateStateStatus = "succeeded"
	PostCreateStateFailed  PostCreateStateStatus = "failed"
)

type PostCreateStatesResult struct {
	PostCreateStates PostCreateStates
	Err              error
}

type PostCreateStates []PostCreateState

type PostCreateState struct {
	Name   string                `json:"name"`
	Status PostCreateStateStatus `json:"status"`
}

func PollPostCreateStates(ctx context.Context, apiClient *api.API, user *api.User, codespace *api.Codespace) (<-chan PostCreateStatesResult, error) {
	pollch := make(chan PostCreateStatesResult)

	token, err := apiClient.GetCodespaceToken(ctx, user.Login, codespace.Name)
	if err != nil {
		return nil, fmt.Errorf("getting codespace token: %v", err)
	}

	lsclient, err := ConnectToLiveshare(ctx, apiClient, user.Login, token, codespace)
	if err != nil {
		return nil, fmt.Errorf("connect to liveshare: %v", err)
	}

	tunnelPort, connClosed, err := MakeSSHTunnel(ctx, lsclient, 0)
	if err != nil {
		return nil, fmt.Errorf("make ssh tunnel: %v", err)
	}

	go func() {
		t := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-connClosed:
				if err != nil {
					pollch <- PostCreateStatesResult{Err: fmt.Errorf("connection closed: %v", err)}
					return
				}
			case <-t.C:
				states, err := getPostCreateOutput(ctx, tunnelPort, codespace)
				if err != nil {
					pollch <- PostCreateStatesResult{Err: fmt.Errorf("get post create output: %v", err)}
					return
				}

				pollch <- PostCreateStatesResult{
					PostCreateStates: states,
				}
			}
		}
	}()

	return pollch, nil
}

func getPostCreateOutput(ctx context.Context, tunnelPort int, codespace *api.Codespace) (PostCreateStates, error) {
	stdout, err := RunCommand(
		ctx, tunnelPort, sshDestination(codespace),
		"cat /workspaces/.codespaces/shared/postCreateOutput.json",
	)
	if err != nil {
		return nil, fmt.Errorf("run command: %v", err)
	}

	b, err := ioutil.ReadAll(stdout)
	if err != nil {
		return nil, fmt.Errorf("read output: %v", err)
	}

	output := struct {
		Steps PostCreateStates `json:"steps"`
	}{}
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
