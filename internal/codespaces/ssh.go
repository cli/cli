package codespaces

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/cli/safeexec"
)

type printer interface {
	Printf(fmt string, v ...interface{})
}

// Shell runs an interactive secure shell over an existing
// port-forwarding session. It runs until the shell is terminated
// (including by cancellation of the context).
func Shell(ctx context.Context, p printer, sshArgs []string, port int, destination string, usingCustomPort bool) error {
	cmd, connArgs, err := newSSHCommand(ctx, port, destination, sshArgs)
	if err != nil {
		return fmt.Errorf("failed to create ssh command: %w", err)
	}

	if usingCustomPort {
		p.Printf("Connection Details: ssh %s %s", destination, connArgs)
	}

	return cmd.Run()
}

// Copy runs an scp command over the specified port. scpArgs should contain both scp flags
// as well as the list of files to copy, with the flags first.
//
// Remote files indicated by a "remote:" prefix are resolved relative
// to the remote user's home directory, and are subject to shell expansion
// on the remote host; see https://lwn.net/Articles/835962/.
func Copy(ctx context.Context, scpArgs []string, port int, destination string) error {
	cmd, err := newSCPCommand(ctx, port, destination, scpArgs)
	if err != nil {
		return fmt.Errorf("failed to create scp command: %w", err)
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
	connArgs := []string{
		"-p", strconv.Itoa(port),
		"-o", "NoHostAuthenticationForLocalhost=yes",
		"-o", "PasswordAuthentication=no",
	}

	// The ssh command syntax is: ssh [flags] user@host command [args...]
	// There is no way to specify the user@host destination as a flag.
	// Unfortunately, that means we need to know which user-provided words are
	// SSH flags and which are command arguments so that we can place
	// them before or after the destination, and that means we need to know all
	// the flags and their arities.
	cmdArgs, command, err := parseSSHArgs(cmdArgs)
	if err != nil {
		return nil, nil, err
	}

	cmdArgs = append(cmdArgs, connArgs...)
	cmdArgs = append(cmdArgs, "-C") // Compression
	cmdArgs = append(cmdArgs, dst)  // user@host

	if command != nil {
		cmdArgs = append(cmdArgs, command...)
	}

	exe, err := safeexec.LookPath("ssh")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute ssh: %w", err)
	}

	cmd := exec.CommandContext(ctx, exe, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	return cmd, connArgs, nil
}

func parseSSHArgs(args []string) (cmdArgs, command []string, err error) {
	return parseArgs(args, "bcDeFIiLlmOopRSWw")
}

// newSCPCommand populates an exec.Cmd to run an scp command for the files specified in cmdArgs.
// cmdArgs is parsed such that scp flags precede the files to copy in the command.
// For example: scp -F ./config local/file remote:file
func newSCPCommand(ctx context.Context, port int, dst string, cmdArgs []string) (*exec.Cmd, error) {
	connArgs := []string{
		"-P", strconv.Itoa(port),
		"-o", "NoHostAuthenticationForLocalhost=yes",
		"-o", "PasswordAuthentication=no",
		"-C", // compression
	}

	cmdArgs, command, err := parseSCPArgs(cmdArgs)
	if err != nil {
		return nil, err
	}

	cmdArgs = append(cmdArgs, connArgs...)

	for _, arg := range command {
		// Replace "remote:" prefix with (e.g.) "root@localhost:".
		if rest := strings.TrimPrefix(arg, "remote:"); rest != arg {
			arg = dst + ":" + rest
		}
		cmdArgs = append(cmdArgs, arg)
	}

	exe, err := safeexec.LookPath("scp")
	if err != nil {
		return nil, fmt.Errorf("failed to execute scp: %w", err)
	}

	// Beware: invalid syntax causes scp to exit 1 with
	// no error message, so don't let that happen.
	cmd := exec.CommandContext(ctx, exe, cmdArgs...)

	cmd.Stdin = nil
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	return cmd, nil
}

func parseSCPArgs(args []string) (cmdArgs, command []string, err error) {
	return parseArgs(args, "cFiJloPS")
}

// parseArgs parses arguments into two distinct slices of flags and command. Parsing stops
// as soon as a non-flag argument is found assuming the remaining arguments are the command.
// It returns an error if a unary flag is provided without an argument.
func parseArgs(args []string, unaryFlags string) (cmdArgs, command []string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// if we've started parsing the command, set it to the rest of the args
		if !strings.HasPrefix(arg, "-") {
			command = args[i:]
			break
		}

		cmdArgs = append(cmdArgs, arg)
		if len(arg) == 2 && strings.Contains(unaryFlags, arg[1:2]) {
			if i++; i == len(args) {
				return nil, nil, fmt.Errorf("flag: %s requires an argument", arg)
			}

			cmdArgs = append(cmdArgs, args[i])
		}
	}

	return cmdArgs, command, nil
}
