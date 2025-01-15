package codespaces

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/internal/codespaces/portforwarder"
	"github.com/cli/cli/v2/internal/codespaces/rpc"
	"github.com/cli/cli/v2/internal/text"
)

// PostCreateStateStatus is a string value representing the different statuses a state can have.
type PostCreateStateStatus string

func (p PostCreateStateStatus) String() string {
	return text.Title(string(p))
}

const (
	PostCreateStateRunning PostCreateStateStatus = "running"
	PostCreateStateSuccess PostCreateStateStatus = "succeeded"
	PostCreateStateFailed  PostCreateStateStatus = "failed"
)

// PostCreateState is a combination of a state and status value that is captured
// during codespace creation.
type PostCreateState struct {
	Name   string                `json:"name"`
	Status PostCreateStateStatus `json:"status"`
}

// PollPostCreateStates watches for state changes in a codespace,
// and calls the supplied poller for each batch of state changes.
// It runs until it encounters an error, including cancellation of the context.
func PollPostCreateStates(ctx context.Context, progress progressIndicator, apiClient apiClient, codespace *api.Codespace, poller func([]PostCreateState)) (err error) {
	codespaceConnection, err := GetCodespaceConnection(ctx, progress, apiClient, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to codespace: %w", err)
	}

	fwd, err := portforwarder.NewPortForwarder(ctx, codespaceConnection)
	if err != nil {
		return fmt.Errorf("failed to create port forwarder: %w", err)
	}
	defer safeClose(fwd, &err)

	// Ensure local port is listening before client (getPostCreateOutput) connects.
	listen, localPort, err := ListenTCP(0, false)
	if err != nil {
		return err
	}

	progress.StartProgressIndicatorWithLabel("Fetching SSH Details")
	invoker, err := rpc.CreateInvoker(ctx, fwd)
	if err != nil {
		return err
	}
	defer safeClose(invoker, &err)

	remoteSSHServerPort, sshUser, err := invoker.StartSSHServer(ctx)
	if err != nil {
		return fmt.Errorf("error getting ssh server details: %w", err)
	}
	progress.StopProgressIndicator()

	progress.StartProgressIndicatorWithLabel("Fetching status")
	tunnelClosed := make(chan error, 1) // buffered to avoid sender stuckness
	go func() {
		opts := portforwarder.ForwardPortOpts{
			Port:     remoteSSHServerPort,
			Internal: true,
		}
		tunnelClosed <- fwd.ForwardPortToListener(ctx, opts, listen)
	}()

	t := time.NewTicker(1 * time.Second)
	defer t.Stop()

	for ticks := 0; ; ticks++ {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case err := <-tunnelClosed:
			return fmt.Errorf("connection failed: %w", err)

		case <-t.C:
			states, err := getPostCreateOutput(ctx, localPort, sshUser)
			// There is an active progress indicator before the first tick
			// to show that we are fetching statuses.
			// Once the first tick happens, we stop the indicator and let
			// the subsequent post create states manage their own progress.
			if ticks == 0 {
				progress.StopProgressIndicator()
			}
			if err != nil {
				return fmt.Errorf("get post create output: %w", err)
			}

			poller(states)
		}
	}
}

func getPostCreateOutput(ctx context.Context, tunnelPort int, user string) ([]PostCreateState, error) {
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
		Steps []PostCreateState `json:"steps"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return nil, fmt.Errorf("unmarshal output: %w", err)
	}

	return output.Steps, nil
}

func safeClose(closer io.Closer, err *error) {
	if closeErr := closer.Close(); *err == nil {
		*err = closeErr
	}
}
