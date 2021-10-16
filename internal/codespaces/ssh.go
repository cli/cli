package codespaces

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/pkg/liveshare"
)

type SSHOptions struct {
	Codespace  string
	Profile    string
	ServerPort int
	Debug      bool
	DebugFile  string
}

// SSH opens an ssh session or runs an ssh command in a codespace.
func (a *App) SSH(ctx context.Context, sshArgs []string, opts SSHOptions) (err error) {
	// Ensure all child tasks (e.g. port forwarding) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	user, err := a.apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %w", err)
	}

	authkeys := make(chan error, 1)
	go func() {
		authkeys <- a.checkAuthorizedKeys(ctx, user.Login)
	}()

	codespace, err := a.getOrChooseCodespace(ctx, opts.Codespace)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %w", err)
	}

	sessionLogger := noopLogger()
	if opts.Debug {
		debugLogger, err := newFileLogger(opts.DebugFile)
		if err != nil {
			return fmt.Errorf("error creating debug logger: %w", err)
		}
		defer safeClose(debugLogger, &err)

		sessionLogger = debugLogger.Logger
		a.logger.Println("Debug file located at: " + debugLogger.Name())
	}

	session, err := a.connectToLiveshare(ctx, sessionLogger, codespace)
	if err != nil {
		return fmt.Errorf("error connecting to Live Share: %w", err)
	}
	defer safeClose(session, &err)

	if err := <-authkeys; err != nil {
		return err
	}

	a.logger.Println("Fetching SSH Details...")
	remoteSSHServerPort, sshUser, err := session.StartSSHServer(ctx)
	if err != nil {
		return fmt.Errorf("error getting ssh server details: %w", err)
	}

	localSSHServerPort := opts.ServerPort
	usingCustomPort := localSSHServerPort != 0 // suppress log of command line in Shell

	// Ensure local port is listening before client (Shell) connects.
	// Unless the user specifies a server port, localSSHServerPort is 0
	// and thus the client will pick a random port.
	listen, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localSSHServerPort))
	if err != nil {
		return err
	}
	defer listen.Close()
	localSSHServerPort = listen.Addr().(*net.TCPAddr).Port

	connectDestination := opts.Profile
	if connectDestination == "" {
		connectDestination = fmt.Sprintf("%s@localhost", sshUser)
	}

	a.logger.Println("Ready...")
	tunnelClosed := make(chan error, 1)
	go func() {
		fwd := liveshare.NewPortForwarder(session, "sshd", remoteSSHServerPort, true)
		tunnelClosed <- fwd.ForwardToListener(ctx, listen) // always non-nil
	}()

	shellClosed := make(chan error, 1)
	go func() {
		shellClosed <- Shell(ctx, a.logger, sshArgs, localSSHServerPort, connectDestination, usingCustomPort)
	}()

	select {
	case err := <-tunnelClosed:
		return fmt.Errorf("tunnel closed: %w", err)
	case err := <-shellClosed:
		if err != nil {
			return fmt.Errorf("shell closed: %w", err)
		}
		return nil // success
	}
}

// fileLogger is a wrapper around an log.Logger configured to write
// to a file. It exports two additional methods to get the log file name
// and close the file handle when the operation is finished.
type fileLogger struct {
	*log.Logger

	f *os.File
}

// newFileLogger creates a new fileLogger. It returns an error if the file
// cannot be created. The file is created on the specified path, if the path
// is empty it is created in the temporary directory.
func newFileLogger(file string) (fl *fileLogger, err error) {
	var f *os.File
	if file == "" {
		f, err = ioutil.TempFile("", "")
		if err != nil {
			return nil, fmt.Errorf("failed to create tmp file: %w", err)
		}
	} else {
		f, err = os.Create(file)
		if err != nil {
			return nil, err
		}
	}

	return &fileLogger{
		Logger: log.New(f, "", log.LstdFlags),
		f:      f,
	}, nil
}

func (fl *fileLogger) Name() string {
	return fl.f.Name()
}

func (fl *fileLogger) Close() error {
	return fl.f.Close()
}

// Shell runs an interactive secure shell over an existing
// port-forwarding session. It runs until the shell is terminated
// (including by cancellation of the context).
func Shell(ctx context.Context, logger *log.Logger, sshArgs []string, port int, destination string, usingCustomPort bool) error {
	cmd, connArgs, err := newSSHCommand(ctx, port, destination, sshArgs)
	if err != nil {
		return fmt.Errorf("failed to create ssh command: %w", err)
	}

	if usingCustomPort {
		logger.Println("Connection Details: ssh " + destination + " " + strings.Join(connArgs, " "))
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
		return nil, nil, err
	}

	cmdArgs = append(cmdArgs, connArgs...)
	cmdArgs = append(cmdArgs, "-C") // Compression
	cmdArgs = append(cmdArgs, dst)  // user@host

	if command != nil {
		cmdArgs = append(cmdArgs, command...)
	}

	cmd := exec.CommandContext(ctx, "ssh", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	return cmd, connArgs, nil
}

// parseSSHArgs parses SSH arguments into two distinct slices of flags and command.
// It returns an error if a unary flag is provided without an argument.
func parseSSHArgs(args []string) (cmdArgs, command []string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// if we've started parsing the command, set it to the rest of the args
		if !strings.HasPrefix(arg, "-") {
			command = args[i:]
			break
		}

		cmdArgs = append(cmdArgs, arg)
		if len(arg) == 2 && strings.Contains("bcDeFIiLlmOopRSWw", arg[1:2]) {
			if i++; i == len(args) {
				return nil, nil, fmt.Errorf("ssh flag: %s requires an argument", arg)
			}

			cmdArgs = append(cmdArgs, args[i])
		}
	}

	return cmdArgs, command, nil
}
