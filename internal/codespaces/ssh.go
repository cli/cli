package codespaces

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

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
