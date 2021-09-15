package codespaces

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/github/go-liveshare"
)

// StartSSHServer installs (if necessary) and starts the SSH in the codespace.
// It returns the remote port where it is running, the user to log in with, or an error if something failed.
func StartSSHServer(ctx context.Context, session *liveshare.Session, log logger) (serverPort int, user string, err error) {
	log.Println("Fetching SSH details...")

	sshServer := session.SSHServer()

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
func Shell(ctx context.Context, log logger, sshArgs []string, port int, destination string, usingCustomPort bool) error {
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
