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
func Shell(ctx context.Context, log logger, sshArgs []string, port int, destination string, usingCustomPort bool) error {
	cmd, connArgs := newSSHCommand(ctx, port, destination, sshArgs)

	if usingCustomPort {
		log.Println("Connection Details: ssh " + destination + " " + strings.Join(connArgs, " "))
	}

	return cmd.Run()
}

// NewRemoteCommand returns an exec.Cmd that will securely run a shell
// command on the remote machine.
func NewRemoteCommand(ctx context.Context, tunnelPort int, destination string, sshArgs ...string) *exec.Cmd {
	cmd, _ := newSSHCommand(ctx, tunnelPort, destination, sshArgs)
	return cmd
}

// newSSHCommand populates an exec.Cmd to run a command (or if blank,
// an interactive shell) over ssh.
func newSSHCommand(ctx context.Context, port int, dst string, cmdArgs []string) (*exec.Cmd, []string) {
	connArgs := []string{"-p", strconv.Itoa(port), "-o", "NoHostAuthenticationForLocalhost=yes"}

	cmdArgs, command := parseSSHArgs(cmdArgs)
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

	return cmd, connArgs
}

var sshArgumentFlags = map[string]bool{
	"-b": true,
	"-c": true,
	"-D": true,
	"-e": true,
	"-F": true,
	"-I": true,
	"-i": true,
	"-L": true,
	"-l": true,
	"-m": true,
	"-O": true,
	"-o": true,
	"-p": true,
	"-R": true,
	"-S": true,
	"-W": true,
	"-w": true,
}

func parseSSHArgs(sshArgs []string) ([]string, string) {
	var (
		cmdArgs      []string
		command      []string
		flagArgument bool
	)

	for _, arg := range sshArgs {
		switch {
		case strings.HasPrefix(arg, "-"):
			cmdArgs = append(cmdArgs, arg)
			if _, ok := sshArgumentFlags[arg]; ok {
				flagArgument = true
			}
		case flagArgument:
			cmdArgs = append(cmdArgs, arg)
			flagArgument = false
		default:
			command = append(command, arg)
		}
	}

	return cmdArgs, strings.Join(command, " ")
}
