package ghcs

import (
	"context"
	"fmt"
	"net"

	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/pkg/liveshare"
	"github.com/spf13/cobra"
)

func newSSHCmd(app *App) *cobra.Command {
	var sshProfile, codespaceName string
	var sshServerPort int

	sshCmd := &cobra.Command{
		Use:   "ssh [flags] [--] [ssh-flags] [command]",
		Short: "SSH into a codespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.SSH(cmd.Context(), args, sshProfile, codespaceName, sshServerPort)
		},
	}

	sshCmd.Flags().StringVarP(&sshProfile, "profile", "", "", "Name of the SSH profile to use")
	sshCmd.Flags().IntVarP(&sshServerPort, "server-port", "", 0, "SSH server port number (0 => pick unused)")
	sshCmd.Flags().StringVarP(&codespaceName, "codespace", "c", "", "Name of the codespace")

	return sshCmd
}

// SSH opens an ssh session or runs an ssh command in a codespace.
func (a *App) SSH(ctx context.Context, sshArgs []string, sshProfile, codespaceName string, localSSHServerPort int) (err error) {
	// Ensure all child tasks (e.g. port forwarding) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	user, err := a.apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %w", err)
	}

	authkeys := make(chan error, 1)
	go func() {
		authkeys <- checkAuthorizedKeys(ctx, a.apiClient, user.Login)
	}()

	codespace, token, err := getOrChooseCodespace(ctx, a.apiClient, user, codespaceName)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %w", err)
	}

	session, err := codespaces.ConnectToLiveshare(ctx, a.logger, a.apiClient, user.Login, token, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to Live Share: %w", err)
	}
	defer safeClose(session, &err)

	if err := <-authkeys; err != nil {
		return err
	}

	a.logger.Println("Fetching SSH Details...")
	remoteSSHServerPort, sshUser, err := session.StartSSHServer(ctx)
	if err != nil {
		return fmt.Errorf("error getting ssh server details: %w", err)
	}

	usingCustomPort := localSSHServerPort != 0 // suppress log of command line in Shell

	// Ensure local port is listening before client (Shell) connects.
	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", localSSHServerPort))
	if err != nil {
		return err
	}
	defer listen.Close()
	localSSHServerPort = listen.Addr().(*net.TCPAddr).Port

	connectDestination := sshProfile
	if connectDestination == "" {
		connectDestination = fmt.Sprintf("%s@localhost", sshUser)
	}

	a.logger.Println("Ready...")
	tunnelClosed := make(chan error, 1)
	go func() {
		fwd := liveshare.NewPortForwarder(session, "sshd", remoteSSHServerPort)
		tunnelClosed <- fwd.ForwardToListener(ctx, listen) // always non-nil
	}()

	shellClosed := make(chan error, 1)
	go func() {
		shellClosed <- codespaces.Shell(ctx, a.logger, sshArgs, localSSHServerPort, connectDestination, usingCustomPort)
	}()

	select {
	case err := <-tunnelClosed:
		return fmt.Errorf("tunnel closed: %w", err)
	case err := <-shellClosed:
		if err != nil {
			return fmt.Errorf("shell closed: %w", err)
		}
		return nil // success
	}
}
