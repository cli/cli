package codespaces

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/github/go-liveshare"
)

func MakeSSHTunnel(ctx context.Context, lsclient *liveshare.Client, localSSHPort int, remoteSSHPort int) (int, <-chan error, error) {
	tunnelClosed := make(chan error)

	server, err := liveshare.NewServer(lsclient)
	if err != nil {
		return 0, nil, fmt.Errorf("new Live Share server: %v", err)
	}

	rand.Seed(time.Now().Unix())
	port := rand.Intn(9999-2000) + 2000 // improve this obviously
	if localSSHPort != 0 {
		port = localSSHPort
	}

	if err := server.StartSharing(ctx, "sshd", remoteSSHPort); err != nil {
		return 0, nil, fmt.Errorf("sharing sshd port: %v", err)
	}

	go func() {
		portForwarder := liveshare.NewPortForwarder(lsclient, server, port)
		if err := portForwarder.Start(ctx); err != nil {
			tunnelClosed <- fmt.Errorf("forwarding port: %v", err)
			return
		}
		tunnelClosed <- nil
	}()

	return port, tunnelClosed, nil
}

// StartSSHServer installs (if necessary) and starts the SSH in the codespace.
// It returns the remote port where it is running, the user to log in with, or an error if something failed.
func StartSSHServer(ctx context.Context, client *liveshare.Client, log logger) (serverPort int, user string, err error) {
	log.Println("Fetching SSH details...")

	sshServer, err := liveshare.NewSSHServer(client)
	if err != nil {
		return 0, "", fmt.Errorf("error creating live share: %v", err)
	}

	sshServerStartResult, err := sshServer.StartRemoteServer(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("error starting live share: %v", err)
	}

	if !sshServerStartResult.Result {
		return 0, "", errors.New(sshServerStartResult.Message)
	}

	portInt, err := strconv.Atoi(sshServerStartResult.ServerPort)
	if err != nil {
		return 0, "", fmt.Errorf("error parsing port: %v", err)
	}

	return portInt, sshServerStartResult.User, nil
}

func makeSSHArgs(port int, dst, cmd string) ([]string, []string) {
	connArgs := []string{"-p", strconv.Itoa(port), "-o", "NoHostAuthenticationForLocalhost=yes"}
	cmdArgs := append([]string{dst, "-X", "-Y", "-C"}, connArgs...) // X11, X11Trust, Compression

	if cmd != "" {
		cmdArgs = append(cmdArgs, cmd)
	}

	return cmdArgs, connArgs
}

func ConnectToTunnel(ctx context.Context, log logger, port int, destination string, usingCustomPort bool) <-chan error {
	connClosed := make(chan error)
	args, connArgs := makeSSHArgs(port, destination, "")

	if usingCustomPort {
		log.Println("Connection Details: ssh " + destination + " " + strings.Join(connArgs, " "))
	}

	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	go func() {
		connClosed <- cmd.Run()
	}()

	return connClosed
}

type command struct {
	Cmd        *exec.Cmd
	StdoutPipe io.ReadCloser
}

func newCommand(cmd *exec.Cmd) (*command, error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cmd start: %v", err)
	}

	return &command{
		Cmd:        cmd,
		StdoutPipe: stdoutPipe,
	}, nil
}

func (c *command) Read(p []byte) (int, error) {
	return c.StdoutPipe.Read(p)
}

func (c *command) Close() error {
	if err := c.StdoutPipe.Close(); err != nil {
		return fmt.Errorf("close stdout: %v", err)
	}

	return c.Cmd.Wait()
}

func RunCommand(ctx context.Context, tunnelPort int, destination, cmdString string) (io.ReadCloser, error) {
	args, _ := makeSSHArgs(tunnelPort, destination, cmdString)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	return newCommand(cmd)
}
