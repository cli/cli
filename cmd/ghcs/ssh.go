package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/github/ghcs/api"
	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/codespaces"
	"github.com/github/go-liveshare"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func newSSHCmd() *cobra.Command {
	var sshProfile, codespaceName string
	var sshServerPort int

	sshCmd := &cobra.Command{
		Use:   "ssh",
		Short: "SSH into a codespace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ssh(context.Background(), sshProfile, codespaceName, sshServerPort)
		},
	}

	sshCmd.Flags().StringVarP(&sshProfile, "profile", "", "", "The `name` of the SSH profile to use")
	sshCmd.Flags().IntVarP(&sshServerPort, "server-port", "", 0, "SSH server port number (0 => pick unused)")
	sshCmd.Flags().StringVarP(&codespaceName, "codespace", "c", "", "The `name` of the codespace to use")

	return sshCmd
}

func init() {
	rootCmd.AddCommand(newSSHCmd())
}

func ssh(ctx context.Context, sshProfile, codespaceName string, localSSHServerPort int) error {
	// Ensure all child tasks (e.g. port forwarding) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	log := output.NewLogger(os.Stdout, os.Stderr, false)

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	codespace, token, err := codespaces.GetOrChooseCodespace(ctx, apiClient, user, codespaceName)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %v", err)
	}

	session, err := codespaces.ConnectToLiveshare(ctx, log, apiClient, user.Login, token, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to Live Share: %v", err)
	}

	remoteSSHServerPort, sshUser, err := codespaces.StartSSHServer(ctx, session, log)
	if err != nil {
		return fmt.Errorf("error getting ssh server details: %v", err)
	}

	terminal := liveshare.NewTerminal(session)

	log.Print("Preparing SSH...")
	if sshProfile == "" {
		containerID, err := getContainerID(ctx, log, terminal)
		if err != nil {
			return fmt.Errorf("error getting container id: %v", err)
		}

		if err := setupEnv(ctx, log, terminal, containerID, codespace.RepositoryName, sshUser); err != nil {
			return fmt.Errorf("error creating ssh server: %v", err)
		}
	}
	log.Print("\n")

	usingCustomPort := localSSHServerPort != 0 // suppress log of command line in Shell

	// Ensure local port is listening before client (Shell) connects.
	listen, err := liveshare.ListenTCP(localSSHServerPort)
	if err != nil {
		return err
	}
	defer listen.Close()
	localSSHServerPort = listen.Addr().(*net.TCPAddr).Port

	connectDestination := sshProfile
	if connectDestination == "" {
		connectDestination = fmt.Sprintf("%s@localhost", sshUser)
	}

	log.Println("Ready...")
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		fwd := liveshare.NewPortForwarder(session, "sshd", remoteSSHServerPort)
		err := fwd.ForwardToListener(ctx, listen) // always non-nil
		return fmt.Errorf("tunnel closed: %v", err)
	})
	group.Go(func() error {
		if err := codespaces.Shell(ctx, log, localSSHServerPort, connectDestination, usingCustomPort); err != nil {
			return fmt.Errorf("shell closed: %v", err)
		}
		return nil // success
	})
	return group.Wait()
}

func getContainerID(ctx context.Context, logger *output.Logger, terminal *liveshare.Terminal) (string, error) {
	logger.Print(".")

	cmd := terminal.NewCommand(
		"/",
		"/usr/bin/docker ps -aq --filter label=Type=codespaces --filter status=running",
	)

	stream, err := cmd.Run(ctx)
	if err != nil {
		return "", fmt.Errorf("error running command: %v", err)
	}

	logger.Print(".")
	scanner := bufio.NewScanner(stream)
	scanner.Scan()

	logger.Print(".")
	containerID := scanner.Text()
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error scanning stream: %v", err)
	}

	logger.Print(".")
	if err := stream.Close(); err != nil {
		return "", fmt.Errorf("error closing stream: %v", err)
	}

	return containerID, nil
}

func setupEnv(ctx context.Context, logger *output.Logger, terminal *liveshare.Terminal, containerID, repositoryName, containerUser string) error {
	setupBashProfileCmd := fmt.Sprintf(`echo "export $(cat /workspaces/.codespaces/shared/.env | xargs); exec /bin/zsh;" > /home/%v/.bash_profile`, containerUser)

	logger.Print(".")
	compositeCommand := []string{setupBashProfileCmd}
	cmd := terminal.NewCommand(
		"/",
		fmt.Sprintf("/usr/bin/docker exec -t %s /bin/bash -c '"+strings.Join(compositeCommand, "; ")+"'", containerID),
	)
	stream, err := cmd.Run(ctx)
	if err != nil {
		return fmt.Errorf("error running command: %v", err)
	}

	logger.Print(".")
	if err := stream.Close(); err != nil {
		return fmt.Errorf("error closing stream: %v", err)
	}

	return nil
}
