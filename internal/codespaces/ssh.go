package codespaces

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/github/go-liveshare"
)

// UnusedPort returns the number of a local TCP port that is currently
// unbound, or an error if none was available.
//
// Use of this function carries an inherent risk of a time-of-check to
// time-of-use race against other processes.
func UnusedPort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, fmt.Errorf("internal error while choosing port: %v", err)
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, fmt.Errorf("choosing available port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// NewPortForwarder returns a new port forwarder for traffic between
// the Live Share client and the specified local and remote ports.
//
// The session name is used (along with the port) to generate
// names for streams, and may appear in error messages.
func NewPortForwarder(ctx context.Context, client *liveshare.Client, sessionName string, localSSHPort, remoteSSHPort int) (*liveshare.PortForwarder, error) {
	if localSSHPort == 0 {
		return nil, fmt.Errorf("a local port must be provided")
	}

	server, err := liveshare.NewServer(client)
	if err != nil {
		return nil, fmt.Errorf("new liveshare server: %v", err)
	}

	if err := server.StartSharing(ctx, "sshd", remoteSSHPort); err != nil {
		return nil, fmt.Errorf("sharing sshd port: %v", err)
	}

	return liveshare.NewPortForwarder(client, server, localSSHPort), nil
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

// Shell runs an interactive secure shell over an existing
// port-forwarding session. It runs until the shell is terminated
// (including by cancellation of the context).
func Shell(ctx context.Context, log logger, port int, destination string, usingCustomPort bool) error {
	cmd, connArgs := newSSHCommand(ctx, port, destination, "")

	if usingCustomPort {
		log.Println("Connection Details: ssh " + destination + " " + strings.Join(connArgs, " "))
	}

	return cmd.Run()
}

// NewRemoteCommand returns an exec.Cmd that will securely run a shell
// command on the remote machine.
func NewRemoteCommand(ctx context.Context, tunnelPort int, destination, command string) *exec.Cmd {
	cmd, _ := newSSHCommand(ctx, tunnelPort, destination, command)
	return cmd
}

// newSSHCommand populates an exec.Cmd to run a command (or if blank,
// an interactive shell) over ssh.
func newSSHCommand(ctx context.Context, port int, dst, command string) (*exec.Cmd, []string) {
	connArgs := []string{"-p", strconv.Itoa(port), "-o", "NoHostAuthenticationForLocalhost=yes"}
	// TODO(adonovan): eliminate X11 and X11Trust flags where unneeded.
	cmdArgs := append([]string{dst, "-X", "-Y", "-C"}, connArgs...) // X11, X11Trust, Compression

	if command != "" {
		cmdArgs = append(cmdArgs, command)
	}

	cmd := exec.CommandContext(ctx, "ssh", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	return cmd, connArgs
}
