package codespaces

import (
	"context"
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

func MakeSSHTunnel(ctx context.Context, lsclient *liveshare.Client, serverPort int) (int, <-chan error, error) {
	tunnelClosed := make(chan error)

	server, err := liveshare.NewServer(lsclient)
	if err != nil {
		return 0, nil, fmt.Errorf("new liveshare server: %v", err)
	}

	rand.Seed(time.Now().Unix())
	port := rand.Intn(9999-2000) + 2000 // improve this obviously
	if serverPort != 0 {
		port = serverPort
	}

	// TODO(josebalius): This port won't always be 2222
	if err := server.StartSharing(ctx, "sshd", 2222); err != nil {
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

func makeSSHArgs(port int, dst, cmd string) ([]string, []string) {
	connArgs := []string{"-p", strconv.Itoa(port), "-o", "NoHostAuthenticationForLocalhost=yes"}
	cmdArgs := append([]string{dst, "-X", "-Y", "-C"}, connArgs...) // X11, X11Trust, Compression

	if cmd != "" {
		cmdArgs = append(cmdArgs, cmd)
	}

	return cmdArgs, connArgs
}

func ConnectToTunnel(ctx context.Context, port int, destination string, usingCustomPort bool) <-chan error {
	connClosed := make(chan error)
	args, connArgs := makeSSHArgs(port, destination, "")

	if usingCustomPort {
		fmt.Println("Connection Details: ssh " + destination + " " + strings.Join(connArgs, " "))
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
