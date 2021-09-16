package codespaces

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Shell runs an interactive secure shell over an existing
// port-forwarding session. It runs until the shell is terminated
// (including by cancellation of the context).
func Shell(ctx context.Context, log logger, sshArgs []string, port int, destination string, usingCustomPort bool) error {
	cmd, connArgs, err := newSSHCommand(ctx, port, destination, sshArgs)
	if err != nil {
		return fmt.Errorf("failed to create ssh command: %w", err)
	}

	if usingCustomPort {
		log.Println("Connection Details: ssh " + destination + " " + strings.Join(connArgs, " "))
	}

	return cmd.Run()
}

// NewRemoteCommand returns an exec.Cmd that will securely run a shell
// command on the remote machine.
func NewRemoteCommand(ctx context.Context, tunnelPort int, destination string, sshArgs ...string) (*exec.Cmd, error) {
	cmd, _, err := newSSHCommand(ctx, tunnelPort, destination, sshArgs)
	return cmd, err
}

// newSSHCommand populates an exec.Cmd to run a command (or if blank,
// an interactive shell) over ssh.
func newSSHCommand(ctx context.Context, port int, dst string, cmdArgs []string) (*exec.Cmd, []string, error) {
	connArgs := []string{"-p", strconv.Itoa(port), "-o", "NoHostAuthenticationForLocalhost=yes"}

	// The ssh command syntax is: ssh [flags] user@host command [args...]
	// There is no way to specify the user@host destination as a flag.
	// Unfortunately, that means we need to know which user-provided words are
	// SSH flags and which are command arguments so that we can place
	// them before or after the destination, and that means we need to know all
	// the flags and their arities.
	cmdArgs, command, err := parseSSHArgs(cmdArgs)
	if err != nil {
		return nil, []string{}, err
	}

	cmdArgs = append(cmdArgs, connArgs...)
	cmdArgs = append(cmdArgs, "-C") // Compression
	cmdArgs = append(cmdArgs, dst)  // user@host

	if command != "" {
		cmdArgs = append(cmdArgs, command)
	}

	cmd := exec.CommandContext(ctx, "ssh", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	return cmd, connArgs, nil
}

var sshArgumentFlags = "-b-c-D-e-F-I-i-L-l-m-O-o-p-R-S-W-w"

func parseSSHArgs(sshArgs []string) ([]string, string, error) {
	var (
		cmdArgs      []string
		command      []string
		flagArgument bool
	)

	for _, arg := range sshArgs {
		switch {
		case strings.HasPrefix(arg, "-"):
			if len(command) > 0 {
				return []string{}, "", fmt.Errorf("invalid flag after command: %s", arg)
			}

			cmdArgs = append(cmdArgs, arg)
			if strings.Contains(sshArgumentFlags, arg) {
				flagArgument = true
			}
		case flagArgument:
			cmdArgs = append(cmdArgs, arg)
			flagArgument = false
		default:
			command = append(command, arg)
		}
	}

	return cmdArgs, strings.Join(command, " "), nil
}
