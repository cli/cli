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

func NewSSHCmd() *cobra.Command {
	var sshProfile, codespaceName string
	var sshServerPort int

	sshCmd := &cobra.Command{
		Use:   "ssh",
		Short: "SSH into a Codespace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return SSH(sshProfile, codespaceName, sshServerPort)
		},
	}

	sshCmd.Flags().StringVarP(&sshProfile, "profile", "", "", "The `name` of the SSH profile to use")
	sshCmd.Flags().IntVarP(&sshServerPort, "server-port", "", 0, "SSH server port number")
	sshCmd.Flags().StringVarP(&codespaceName, "codespace", "c", "", "The `name` of the Codespace to use")

	return sshCmd
}

func init() {
	rootCmd.AddCommand(NewSSHCmd())
}

func SSH(sshProfile, codespaceName string, sshServerPort int) error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()
	log := output.NewLogger(os.Stdout, os.Stderr, false)

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	codespace, token, err := codespaces.GetOrChooseCodespace(ctx, apiClient, user, codespaceName)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %v", err)
	}

	lsclient, err := codespaces.ConnectToLiveshare(ctx, log, apiClient, token, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to liveshare: %v", err)
	}

	result, remoteSSHServerPort, sshUser, _, err := codespaces.StartSSHServer(ctx, lsclient)
	if err != nil {
		return fmt.Errorf("error getting ssh server details: %v", err)
	}

	if !result {
		return fmt.Errorf("error starting ssh: %v", err)
	}

	tunnelPort, tunnelClosed, err := codespaces.MakeSSHTunnel(ctx, lsclient, sshServerPort, remoteSSHServerPort)
	if err != nil {
		return fmt.Errorf("make ssh tunnel: %v", err)
	}

	connectDestination := sshProfile
	if connectDestination == "" {
		connectDestination = fmt.Sprintf("%s@localhost", sshUser)
	}

	usingCustomPort := tunnelPort == sshServerPort
	connClosed := codespaces.ConnectToTunnel(ctx, log, tunnelPort, connectDestination, usingCustomPort)

	log.Println("Ready...")
	select {
	case err := <-tunnelClosed:
		if err != nil {
			return fmt.Errorf("tunnel closed: %v", err)
		}
	case err := <-connClosed:
		if err != nil {
			return fmt.Errorf("connection closed: %v", err)
		}
	}

	return nil
}
