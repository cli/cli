package codespace

// This file defines the 'gh cs ssh' and 'gh cs cp' subcommands.

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/pkg/liveshare"
	"github.com/spf13/cobra"
)

type sshOptions struct {
	codespace  string
	profile    string
	serverPort int
	debug      bool
	debugFile  string
	scpArgs    []string // scp arguments, for 'cs cp' (nil for 'cs ssh')
}

func newSSHCmd(app *App) *cobra.Command {
	var opts sshOptions

	sshCmd := &cobra.Command{
		Use:   "ssh [flags] [--] [ssh-flags] [command]",
		Short: "SSH into a codespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.SSH(cmd.Context(), args, opts)
		},
	}

	sshCmd.Flags().StringVarP(&opts.profile, "profile", "", "", "Name of the SSH profile to use")
	sshCmd.Flags().IntVarP(&opts.serverPort, "server-port", "", 0, "SSH server port number (0 => pick unused)")
	sshCmd.Flags().StringVarP(&opts.codespace, "codespace", "c", "", "Name of the codespace")
	sshCmd.Flags().BoolVarP(&opts.debug, "debug", "d", false, "Log debug data to a file")
	sshCmd.Flags().StringVarP(&opts.debugFile, "debug-file", "", "", "Path of the file log to")

	return sshCmd
}

// SSH opens an ssh session or runs an ssh command in a codespace.
func (a *App) SSH(ctx context.Context, sshArgs []string, opts sshOptions) (err error) {
	// Ensure all child tasks (e.g. port forwarding) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	user, err := a.apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %w", err)
	}

	authkeys := make(chan error, 1)
	go func() {
		authkeys <- checkAuthorizedKeys(ctx, a.apiClient, user.Login)
	}()

	codespace, err := getOrChooseCodespace(ctx, a.apiClient, opts.codespace)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %w", err)
	}

	liveshareLogger := noopLogger()
	if opts.debug {
		debugLogger, err := newFileLogger(opts.debugFile)
		if err != nil {
			return fmt.Errorf("error creating debug logger: %w", err)
		}
		defer safeClose(debugLogger, &err)

		liveshareLogger = debugLogger.Logger
		a.logger.Println("Debug file located at: " + debugLogger.Name())
	}

	session, err := codespaces.ConnectToLiveshare(ctx, a.logger, liveshareLogger, a.apiClient, codespace)
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

	localSSHServerPort := opts.serverPort
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

	connectDestination := opts.profile
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
		var err error
		if opts.scpArgs != nil {
			err = codespaces.Copy(ctx, opts.scpArgs, localSSHServerPort, connectDestination)
		} else {
			err = codespaces.Shell(ctx, a.logger, sshArgs, localSSHServerPort, connectDestination, usingCustomPort)
		}
		shellClosed <- err
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

type cpOptions struct {
	sshOptions
	recursive bool // -r
}

func newCpCmd(app *App) *cobra.Command {
	var opts cpOptions

	cpCmd := &cobra.Command{
		Use:   "cp [-r] srcs... dest",
		Short: "Copy files between local and remote file systems",
		Long: `
The cp command copies files between the local and remote file systems.

A 'remote:' prefix on any file name argument indicates that it refers to
the file system of the remote (Codespace) machine. It is resolved relative
to the home directory of the remote user.

As with the UNIX cp command, the first argument specifies the source and the last
specifies the destination; additional sources may be specified after the first,
if the destination is a directory.

The -r (recursive) flag is required if any source is a directory.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Copy(cmd.Context(), args, opts)
		},
	}

	// We don't expose all sshOptions.
	cpCmd.Flags().BoolVarP(&opts.recursive, "recursive", "r", false, "Recursively copy directories")
	cpCmd.Flags().StringVarP(&opts.codespace, "codespace", "c", "", "Name of the codespace")
	return cpCmd
}

// Copy copies files between the local and remote file systems.
// The mechanics are similar to 'ssh' but using 'scp'.
func (a *App) Copy(ctx context.Context, args []string, opts cpOptions) (err error) {
	if len(args) < 2 {
		return fmt.Errorf("cp requires source and destination arguments")
	}
	if opts.recursive {
		opts.scpArgs = append(opts.scpArgs, "-r")
	}
	opts.scpArgs = append(opts.scpArgs, "--")
	for _, arg := range args {
		if !filepath.IsAbs(arg) && !strings.HasPrefix(arg, "remote:") {
			// scp treats a colon in the first path segment as a host identifier.
			// Escape it by prepending "./".
			// TODO(adonovan): test on Windows, including with a c:\\foo path.
			const sep = string(os.PathSeparator)
			first := strings.Split(filepath.ToSlash(arg), sep)[0]
			if strings.Contains(first, ":") {
				arg = "." + sep + arg
			}
		}
		opts.scpArgs = append(opts.scpArgs, arg)
	}
	return a.SSH(ctx, nil, opts.sshOptions)
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
