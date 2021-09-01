package main

import (
	"context"
	"fmt"
	"os"

	"github.com/github/ghcs/api"
	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/codespaces"
	"github.com/spf13/cobra"
)

func newLogsCmd() *cobra.Command {
	var tail bool

	logsCmd := &cobra.Command{
		Use:   "logs [<codespace>]",
		Short: "Access codespace logs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var codespaceName string
			if len(args) > 0 {
				codespaceName = args[0]
			}
			return logs(context.Background(), tail, codespaceName)
		},
	}

	logsCmd.Flags().BoolVarP(&tail, "tail", "t", false, "Tail the logs")

	return logsCmd
}

func init() {
	rootCmd.AddCommand(newLogsCmd())
}

func logs(ctx context.Context, tail bool, codespaceName string) error {
	// Ensure all child tasks (port forwarding, remote exec) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	log := output.NewLogger(os.Stdout, os.Stderr, false)

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("getting user: %v", err)
	}

	codespace, token, err := codespaces.GetOrChooseCodespace(ctx, apiClient, user, codespaceName)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %v", err)
	}

	lsclient, err := codespaces.ConnectToLiveshare(ctx, log, apiClient, user.Login, token, codespace)
	if err != nil {
		return fmt.Errorf("connecting to Live Share: %v", err)
	}

	localSSHPort, err := codespaces.UnusedPort()
	if err != nil {
		return err
	}

	remoteSSHServerPort, sshUser, err := codespaces.StartSSHServer(ctx, lsclient, log)
	if err != nil {
		return fmt.Errorf("error getting ssh server details: %v", err)
	}

	tunnel, err := codespaces.NewPortForwarder(ctx, lsclient, "sshd", localSSHPort, remoteSSHServerPort)
	if err != nil {
		return fmt.Errorf("make ssh tunnel: %v", err)
	}

	cmdType := "cat"
	if tail {
		cmdType = "tail -f"
	}

	dst := fmt.Sprintf("%s@localhost", sshUser)
	cmd := codespaces.NewRemoteCommand(
		ctx, localSSHPort, dst, fmt.Sprintf("%s /workspaces/.codespaces/.persistedshare/creation.log", cmdType),
	)

	// Error channels are buffered so that neither sending goroutine gets stuck.

	tunnelClosed := make(chan error, 1)
	go func() {
		tunnelClosed <- tunnel.Start(ctx) // error is non-nil
	}()

	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.Run()
	}()

	select {
	case err := <-tunnelClosed:
		return fmt.Errorf("connection closed: %v", err)

	case err := <-cmdDone:
		if err != nil {
			return fmt.Errorf("error retrieving logs: %v", err)
		}
		return nil // success
	}
}
