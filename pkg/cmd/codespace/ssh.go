package codespace

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"

	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/pkg/cmd/codespace/output"
	"github.com/cli/cli/v2/pkg/liveshare"
	"github.com/spf13/cobra"
)

type sshOptions struct {
	codespace  string
	profile    string
	serverPort int
	debug      bool
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

	var debugLogger *fileLogger
	if opts.debug {
		debugLogger, err = newFileLogger("gh-cs-ssh")
		if err != nil {
			return fmt.Errorf("error creating debug logger: %w", err)
		}
		defer safeClose(debugLogger, &err)
		a.logger.Println("Debug file located at: " + debugLogger.Name())
	}

	session, err := codespaces.ConnectToLiveshare(ctx, a.logger, debugLogger, a.apiClient, codespace)
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
	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", localSSHServerPort))
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
		shellClosed <- codespaces.Shell(ctx, a.logger, sshArgs, localSSHServerPort, connectDestination, usingCustomPort)
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

// fileLogger is a wrapper around an output.Logger configured to write
// to a file. It exports two additional methods to get the log file name
// and close the file handle when the operation is finished.
type fileLogger struct {
	// TODO(josebalius): should we use https://pkg.go.dev/log#New instead?
	*output.Logger

	f *os.File
}

// newFileLogger creates a new fileLogger. It returns an error if the file
// cannot be created. The file is created in the operating system tmp directory
// under the name parameter.
func newFileLogger(name string) (*fileLogger, error) {
	f, err := ioutil.TempFile("", name)
	if err != nil {
		return nil, err
	}
	return &fileLogger{
		Logger: output.NewLogger(f, f, false),
		f:      f,
	}, nil
}

func (fl *fileLogger) Name() string {
	return fl.f.Name()
}

func (fl *fileLogger) Close() error {
	return fl.f.Close()
}
