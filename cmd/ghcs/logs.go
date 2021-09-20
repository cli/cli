package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/api"
	"github.com/github/ghcs/internal/codespaces"
	"github.com/github/go-liveshare"
	"github.com/spf13/cobra"
)

func newLogsCmd() *cobra.Command {
	var (
		codespace string
		follow    bool
	)

	log := output.NewLogger(os.Stdout, os.Stderr, false)

	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Access codespace logs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return logs(context.Background(), log, codespace, follow)
		},
	}

	logsCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
	logsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "Tail and follow the logs")

	return logsCmd
}

func init() {
	rootCmd.AddCommand(newLogsCmd())
}

func logs(ctx context.Context, log *output.Logger, codespaceName string, follow bool) error {
	// Ensure all child tasks (port forwarding, remote exec) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("getting user: %w", err)
	}

	codespace, token, err := getOrChooseCodespace(ctx, apiClient, user, codespaceName)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %w", err)
	}

	session, err := codespaces.ConnectToLiveshare(ctx, log, apiClient, user.Login, token, codespace)
	if err != nil {
		return fmt.Errorf("connecting to Live Share: %w", err)
	}

	// Ensure local port is listening before client (getPostCreateOutput) connects.
	listen, err := net.Listen("tcp", ":0") // arbitrary port
	if err != nil {
		return err
	}
	defer listen.Close()
	localPort := listen.Addr().(*net.TCPAddr).Port

	log.Println("Fetching SSH Details...")
	remoteSSHServerPort, sshUser, err := session.StartSSHServer(ctx)
	if err != nil {
		return fmt.Errorf("error getting ssh server details: %w", err)
	}

	cmdType := "cat"
	if follow {
		cmdType = "tail -f"
	}

	dst := fmt.Sprintf("%s@localhost", sshUser)
	cmd, err := codespaces.NewRemoteCommand(
		ctx, localPort, dst, fmt.Sprintf("%s /workspaces/.codespaces/.persistedshare/creation.log", cmdType),
	)
	if err != nil {
		return fmt.Errorf("remote command: %w", err)
	}

	tunnelClosed := make(chan error, 1)
	go func() {
		fwd := liveshare.NewPortForwarder(session, "sshd", remoteSSHServerPort)
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
