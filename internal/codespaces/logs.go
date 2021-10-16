package codespaces

import (
	"context"
	"fmt"
	"net"

	"github.com/cli/cli/v2/pkg/liveshare"
)

func (a *App) Logs(ctx context.Context, codespaceName string, follow bool) (err error) {
	// Ensure all child tasks (port forwarding, remote exec) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	user, err := a.apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("getting user: %w", err)
	}

	authkeys := make(chan error, 1)
	go func() {
		authkeys <- a.checkAuthorizedKeys(ctx, user.Login)
	}()

	codespace, err := a.getOrChooseCodespace(ctx, codespaceName)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %w", err)
	}

	session, err := a.connectToLiveshare(ctx, noopLogger(), codespace)
	if err != nil {
		return fmt.Errorf("connecting to Live Share: %w", err)
	}
	defer safeClose(session, &err)

	if err := <-authkeys; err != nil {
		return err
	}

	// Ensure local port is listening before client (getPostCreateOutput) connects.
	listen, err := net.Listen("tcp", "127.0.0.1:0") // arbitrary port
	if err != nil {
		return err
	}
	defer listen.Close()
	localPort := listen.Addr().(*net.TCPAddr).Port

	a.logger.Println("Fetching SSH Details...")
	remoteSSHServerPort, sshUser, err := session.StartSSHServer(ctx)
	if err != nil {
		return fmt.Errorf("error getting ssh server details: %w", err)
	}

	cmdType := "cat"
	if follow {
		cmdType = "tail -f"
	}

	dst := fmt.Sprintf("%s@localhost", sshUser)
	cmd, err := NewRemoteCommand(
		ctx, localPort, dst, fmt.Sprintf("%s /workspaces/.codespaces/.persistedshare/creation.log", cmdType),
	)
	if err != nil {
		return fmt.Errorf("remote command: %w", err)
	}

	tunnelClosed := make(chan error, 1)
	go func() {
		fwd := liveshare.NewPortForwarder(session, "sshd", remoteSSHServerPort, false)
		tunnelClosed <- fwd.ForwardToListener(ctx, listen) // error is non-nil
	}()

	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.Run()
	}()

	select {
	case err := <-tunnelClosed:
		return fmt.Errorf("connection closed: %w", err)
	case err := <-cmdDone:
		if err != nil {
			return fmt.Errorf("error retrieving logs: %w", err)
		}

		return nil // success
	}
}
