package main

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/github/ghcs/api"
	"github.com/github/ghcs/internal/codespaces"
	"github.com/github/go-liveshare"
	"github.com/spf13/cobra"
)

func NewSSHCmd() *cobra.Command {
	var sshProfile string
	var sshServerPort int

	sshCmd := &cobra.Command{
		Use:   "ssh",
		Short: "SSH into a GitHub Codespace, for use with running tests/editing in vim, etc.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return SSH(sshProfile, sshServerPort)
		},
	}

	sshCmd.Flags().StringVarP(&sshProfile, "profile", "", "", "SSH Profile")
	sshCmd.Flags().IntVarP(&sshServerPort, "server-port", "", 0, "SSH Server Port")

	return sshCmd
}

func init() {
	rootCmd.AddCommand(NewSSHCmd())
}

func SSH(sshProfile string, sshServerPort int) error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	codespace, err := codespaces.ChooseCodespace(ctx, apiClient, user)
	if err != nil {
		if err == codespaces.ErrNoCodespaces {
			fmt.Println(err.Error())
			return nil
		}

		return fmt.Errorf("error choosing codespace: %v", err)
	}

	token, err := apiClient.GetCodespaceToken(ctx, user.Login, codespace.Name)
	if err != nil {
		return fmt.Errorf("error getting codespace token: %v", err)
	}

	liveShareClient, err := codespaces.ConnectToLiveshare(ctx, apiClient, token, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to liveshare: %v", err)
	}

	terminal, err := liveShareClient.NewTerminal()
	if err != nil {
		return fmt.Errorf("error creating liveshare terminal: %v", err)
	}

	fmt.Println("Preparing SSH...")
	if sshProfile == "" {
		containerID, err := getContainerID(ctx, terminal)
		if err != nil {
			return fmt.Errorf("error getting container id: %v", err)
		}

		if err := setupSSH(ctx, terminal, containerID, codespace.RepositoryName); err != nil {
			return fmt.Errorf("error creating ssh server: %v", err)
		}

		fmt.Printf("\n")
	}

	server, err := liveShareClient.NewServer()
	if err != nil {
		return fmt.Errorf("error creating server: %v", err)
	}

	rand.Seed(time.Now().Unix())
	port := rand.Intn(9999-2000) + 2000 // improve this obviously
	if sshServerPort != 0 {
		port = sshServerPort
	}

	if err := server.StartSharing(ctx, "sshd", 2222); err != nil {
		return fmt.Errorf("error sharing sshd port: %v", err)
	}

	portForwarder := liveshare.NewLocalPortForwarder(liveShareClient, server, port)
	go func() {
		if err := portForwarder.Start(ctx); err != nil {
			panic(fmt.Errorf("error forwarding port: %v", err))
		}
	}()

	connectDestination := sshProfile
	if connectDestination == "" {
		connectDestination = fmt.Sprintf("%s@localhost", getSSHUser(codespace))
	}

	fmt.Println("Ready...")
	if err := connect(ctx, port, connectDestination, port == sshServerPort); err != nil {
		return fmt.Errorf("error connecting via SSH: %v", err)
	}

	return nil
}

func connect(ctx context.Context, port int, destination string, setServerPort bool) error {
	connectionDetailArgs := []string{"-p", strconv.Itoa(port), "-o", "NoHostAuthenticationForLocalhost=yes"}

	if setServerPort {
		fmt.Println("Connection Details: ssh " + destination + " " + strings.Join(connectionDetailArgs, " "))
	}

	args := []string{destination, "-X", "-Y", "-C"} // X11, X11Trust, Compression
	cmd := exec.CommandContext(ctx, "ssh", append(args, connectionDetailArgs...)...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func getContainerID(ctx context.Context, terminal *liveshare.Terminal) (string, error) {
	fmt.Print(".")
	cmd := terminal.NewCommand(
		"/",
		"/usr/bin/docker ps -aq --filter label=Type=codespaces --filter status=running",
	)
	stream, err := cmd.Run(ctx)
	if err != nil {
		return "", fmt.Errorf("error running command: %v", err)
	}

	fmt.Print(".")
	scanner := bufio.NewScanner(stream)
	scanner.Scan()

	fmt.Print(".")
	containerID := scanner.Text()
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error scanning stream: %v", err)
	}

	fmt.Print(".")
	if err := stream.Close(); err != nil {
		return "", fmt.Errorf("error closing stream: %v", err)
	}

	return containerID, nil
}

func setupSSH(ctx context.Context, terminal *liveshare.Terminal, containerID, repositoryName string) error {
	setupBashProfileCmd := fmt.Sprintf(`echo "cd /workspaces/%v; export $(cat /workspaces/.codespaces/shared/.env | xargs); exec /bin/zsh;" > /home/codespace/.bash_profile`, repositoryName)

	fmt.Print(".")
	compositeCommand := []string{setupBashProfileCmd}
	cmd := terminal.NewCommand(
		"/",
		fmt.Sprintf("/usr/bin/docker exec -t %s /bin/bash -c '"+strings.Join(compositeCommand, "; ")+"'", containerID),
	)
	stream, err := cmd.Run(ctx)
	if err != nil {
		return fmt.Errorf("error running command: %v", err)
	}

	fmt.Print(".")
	if err := stream.Close(); err != nil {
		return fmt.Errorf("error closing stream: %v", err)
	}

	time.Sleep(1 * time.Second)

	return nil
}

func getSSHUser(codespace *api.Codespace) string {
	if codespace.RepositoryNWO == "github/github" {
		return "root"
	}
	return "codespace"
}
