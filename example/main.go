package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/github/go-liveshare"
)

var workspaceIdFlag = flag.String("w", "", "workspace session id")

func init() {
	flag.Parse()
}

func main() {
	liveShare, err := liveshare.New(
		liveshare.WithWorkspaceID(*workspaceIdFlag),
		liveshare.WithToken(os.Getenv("CODESPACE_TOKEN")),
	)
	if err != nil {
		log.Fatal(fmt.Errorf("error creating liveshare: %v", err))
	}

	ctx := context.Background()
	liveShareClient := liveShare.NewClient()
	if err := liveShareClient.Join(ctx); err != nil {
		log.Fatal(fmt.Errorf("error joining liveshare with client: %v", err))
	}

	terminal, err := liveShareClient.NewTerminal()
	if err != nil {
		log.Fatal(fmt.Errorf("error creating liveshare terminal"))
	}

	containerID, err := getContainerID(ctx, terminal)
	if err != nil {
		log.Fatal(fmt.Errorf("error getting container id: %v", err))
	}

	if err := setupSSH(ctx, terminal, containerID); err != nil {
		log.Fatal(fmt.Errorf("error setting up ssh: %v", err))
	}

	fmt.Println("Starting server...")

	server, err := liveShareClient.NewServer()
	if err != nil {
		log.Fatal(fmt.Errorf("error creating server: %v", err))
	}

	fmt.Println("Starting sharing...")
	if err := server.StartSharing(ctx, "sshd", 2222); err != nil {
		log.Fatal(fmt.Errorf("error server sharing: %v", err))
	}

	portForwarder := liveshare.NewLocalPortForwarder(liveShareClient, server, 2222)

	fmt.Println("Listening on port 2222")
	if err := portForwarder.Start(ctx); err != nil {
		log.Fatal(fmt.Errorf("error forwarding port: %v", err))
	}
}

func setupSSH(ctx context.Context, terminal *liveshare.Terminal, containerID string) error {
	cmd := terminal.NewCommand(
		"/",
		fmt.Sprintf("/usr/bin/docker exec -t %s /bin/bash -c \"echo -e \\\"testpwd1\\ntestpwd1\\n\\\" | sudo passwd codespace;/usr/local/share/ssh-init.sh\"", containerID),
	)
	stream, err := cmd.Run(ctx)
	if err != nil {
		return fmt.Errorf("error running command: %v", err)
	}

	scanner := bufio.NewScanner(stream)
	scanner.Scan()

	fmt.Println("> Debug:", scanner.Text())
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error scanning stream: %v", err)
	}

	if err := stream.Close(); err != nil {
		return fmt.Errorf("error closing stream: %v", err)
	}

	time.Sleep(2 * time.Second)

	return nil
}

func getContainerID(ctx context.Context, terminal *liveshare.Terminal) (string, error) {
	cmd := terminal.NewCommand(
		"/",
		"/usr/bin/docker ps -aq --filter label=Type=codespaces --filter status=running",
	)
	stream, err := cmd.Run(ctx)
	if err != nil {
		return "", fmt.Errorf("error running command: %v", err)
	}

	scanner := bufio.NewScanner(stream)
	scanner.Scan()

	containerID := scanner.Text()
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error scanning stream: %v", err)
	}

	if err := stream.Close(); err != nil {
		return "", fmt.Errorf("error closing stream: %v", err)
	}

	return containerID, nil
}
