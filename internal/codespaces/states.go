package codespaces

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/liveshare"
)

// postCreateStateStatus is a string value representing the different statuses a state can have.
type postCreateStateStatus string

func (p postCreateStateStatus) String() string {
	return strings.Title(string(p))
}

const (
	postCreateStateRunning postCreateStateStatus = "running"
	postCreateStateSuccess postCreateStateStatus = "succeeded"
	postCreateStateFailed  postCreateStateStatus = "failed"
)

// postCreateState is a combination of a state and status value that is captured
// during codespace creation.
type postCreateState struct {
	Name   string                `json:"name"`
	Status postCreateStateStatus `json:"status"`
}

// pollPostCreateStates watches for state changes in a codespace,
// and calls the supplied poller for each batch of state changes.
// It runs until it encounters an error, including cancellation of the context.
func (a *App) pollPostCreateStates(ctx context.Context, codespace *api.Codespace, poller func([]postCreateState)) (err error) {
	noopLogger := log.New(ioutil.Discard, "", 0)

	session, err := a.connectToLiveshare(ctx, noopLogger, codespace)
	if err != nil {
		return fmt.Errorf("connect to Live Share: %w", err)
	}
	defer func() {
		if closeErr := session.Close(); err == nil {
			err = closeErr
		}
	}()

	// Ensure local port is listening before client (getPostCreateOutput) connects.
	listen, err := net.Listen("tcp", "127.0.0.1:0") // arbitrary port
	if err != nil {
		return err
	}
	localPort := listen.Addr().(*net.TCPAddr).Port

	a.logger.Println("Fetching SSH Details...")
	remoteSSHServerPort, sshUser, err := session.StartSSHServer(ctx)
	if err != nil {
		return fmt.Errorf("error getting ssh server details: %w", err)
	}

	tunnelClosed := make(chan error, 1) // buffered to avoid sender stuckness
	go func() {
		fwd := liveshare.NewPortForwarder(session, "sshd", remoteSSHServerPort, false)
		tunnelClosed <- fwd.ForwardToListener(ctx, listen) // error is non-nil
	}()

	t := time.NewTicker(1 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case err := <-tunnelClosed:
			return fmt.Errorf("connection failed: %w", err)

		case <-t.C:
			states, err := getPostCreateOutput(ctx, localPort, sshUser)
			if err != nil {
				return fmt.Errorf("get post create output: %w", err)
			}

			poller(states)
		}
	}
}

func getPostCreateOutput(ctx context.Context, tunnelPort int, user string) ([]postCreateState, error) {
	cmd, err := NewRemoteCommand(
		ctx, tunnelPort, fmt.Sprintf("%s@localhost", user),
		"cat /workspaces/.codespaces/shared/postCreateOutput.json",
	)
	if err != nil {
		return nil, fmt.Errorf("remote command: %w", err)
	}

	stdout := new(bytes.Buffer)
	cmd.Stdout = stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run command: %w", err)
	}
	var output struct {
		Steps []postCreateState `json:"steps"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return nil, fmt.Errorf("unmarshal output: %w", err)
	}

	return output.Steps, nil
}
