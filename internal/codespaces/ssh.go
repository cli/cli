package codespaces

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/cli/safeexec"
	"github.com/google/shlex"
)

type printer interface {
	Printf(fmt string, v ...any)
}

// Shell runs an interactive secure shell over an existing
// port-forwarding session. It runs until the shell is terminated
// (including by cancellation of the context).
func Shell(
	ctx context.Context, p printer, sshArgs []string, command []string, port int, destination string, printConnDetails bool,
) error {
	cmd, connArgs, err := newSSHCommand(ctx, port, destination, sshArgs, command)
	if err != nil {
		return fmt.Errorf("failed to create ssh command: %w", err)
	}

	if printConnDetails {
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
	sshArgs, command, err := ParseSSHArgs(sshArgs)
	if err != nil {
		return nil, err
	}

	cmd, _, err := newSSHCommand(ctx, tunnelPort, destination, sshArgs, command)
	return cmd, err
}

// newSSHCommand populates an exec.Cmd to run a command (or if blank,
// an interactive shell) over ssh.
func newSSHCommand(ctx context.Context, port int, dst string, cmdArgs []string, command []string) (*exec.Cmd, []string, error) {
	connArgs := []string{
		"-p", strconv.Itoa(port),
		"-o", "NoHostAuthenticationForLocalhost=yes",
		"-o", "PasswordAuthentication=no",
	}

	cmdArgs = append(cmdArgs, connArgs...)
	cmdArgs = append(cmdArgs, "-C") // Compression
	cmdArgs = append(cmdArgs, dst)  // user@host

	if command != nil {
		cmdArgs = append(cmdArgs, command...)
	}

	exe, args, err := getSSHCommand()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute ssh: %w", err)
	}

	// If we have custom args from the environment variable, insert them at the beginning
	if len(args) > 0 {
		cmdArgs = append(args, cmdArgs...)
	}

	cmd := exec.CommandContext(ctx, exe, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	return cmd, connArgs, nil
}

// ParseSSHArgs parses the given array of arguments into two distinct slices of flags and command.
// The ssh command syntax is: ssh [flags] user@host command [args...]
// There is no way to specify the user@host destination as a flag.
// Unfortunately, that means we need to know which user-provided words are
// SSH flags and which are command arguments so that we can place
// them before or after the destination, and that means we need to know all
// the flags and their arities.
func ParseSSHArgs(args []string) (cmdArgs, command []string, err error) {
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
		if rest, ok := strings.CutPrefix(arg, "remote:"); ok {
			arg = dst + ":" + rest
		}
		cmdArgs = append(cmdArgs, arg)
	}

	exe, args, err := getSCPCommand()
	if err != nil {
		return nil, fmt.Errorf("failed to execute scp: %w", err)
	}

	// If we have custom args from the environment variable, insert them at the beginning
	if len(args) > 0 {
		cmdArgs = append(args, cmdArgs...)
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

// getCommandFromEnv returns the path to a command and any additional arguments based on an environment variable.
// If the environment variable is set, its value is parsed to extract both the command path and any additional arguments.
// Otherwise, it looks for the default command in the PATH.
func getCommandFromEnv(envVar, defaultCmd string) (string, []string, error) {
	if cmdLine := os.Getenv(envVar); cmdLine != "" {
		// Parse the command line respecting quotes using shlex
		args, err := shlex.Split(cmdLine)
		if err != nil {
			return "", nil, fmt.Errorf("invalid %s: %w", envVar, err)
		}
		if len(args) == 0 {
			return "", nil, fmt.Errorf("empty %s", envVar)
		}

		// If the command is a path, use it directly
		// Otherwise, use safeexec.LookPath to find the executable
		cmd := args[0]
		if !strings.Contains(cmd, "/") {
			cmd, err = safeexec.LookPath(cmd)
			if err != nil {
				return "", nil, fmt.Errorf("failed to find command '%s' specified in %s: %w", args[0], envVar, err)
			}
		}

		return cmd, args[1:], nil
	}

	cmd, err := safeexec.LookPath(defaultCmd)
	return cmd, nil, err
}

// getSSHCommand returns the path to the SSH command to use and any additional arguments.
// If the GH_CS_SSH_COMMAND environment variable is set, its value is parsed to extract
// both the command path and any additional arguments.
// Otherwise, it looks for the "ssh" command in the PATH.
// Similar to GIT_SSH_COMMAND, this supports specifying both the command path and additional arguments.
func getSSHCommand() (string, []string, error) {
	return getCommandFromEnv("GH_CS_SSH_COMMAND", "ssh")
}

// getSCPCommand returns the path to the SCP command to use and any additional arguments.
// If the GH_CS_SCP_COMMAND environment variable is set, its value is parsed to extract
// both the command path and any additional arguments.
// Otherwise, it looks for the "scp" command in the PATH.
// Similar to GIT_SSH_COMMAND, this supports specifying both the command path and additional arguments.
func getSCPCommand() (string, []string, error) {
	return getCommandFromEnv("GH_CS_SCP_COMMAND", "scp")
}
