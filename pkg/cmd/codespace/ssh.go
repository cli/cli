package codespace

// This file defines the 'gh cs ssh' and 'gh cs cp' subcommands.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/internal/codespaces/rpc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/liveshare"
	"github.com/cli/cli/v2/pkg/ssh"
	"github.com/cli/safeexec"
	"github.com/spf13/cobra"
)

// In 2.13.0 these commands started automatically generating key pairs named 'codespaces' and 'codespaces.pub'
// which could collide with suggested the ssh config also named 'codespaces'. We now use 'codespaces.auto'
// and 'codespaces.auto.pub' in order to avoid that collision.
const automaticPrivateKeyNameOld = "codespaces"
const automaticPrivateKeyName = "codespaces.auto"

var errKeyFileNotFound = errors.New("SSH key file does not exist")

type sshOptions struct {
	selector   *CodespaceSelector
	profile    string
	serverPort int
	debug      bool
	debugFile  string
	stdio      bool
	config     bool
	scpArgs    []string // scp arguments, for 'cs cp' (nil for 'cs ssh')
}

func newSSHCmd(app *App) *cobra.Command {
	var opts sshOptions

	sshCmd := &cobra.Command{
		Use:   "ssh [<flags>...] [-- <ssh-flags>...] [<command>]",
		Short: "SSH into a codespace",
		Long: heredoc.Doc(`
			The 'ssh' command is used to SSH into a codespace. In its simplest form, you can
			run 'gh cs ssh', select a codespace interactively, and connect.
			
			By default, the 'ssh' command will create a public/private ssh key pair to  
			authenticate with the codespace inside the ~/.ssh directory.

			The 'ssh' command also supports deeper integration with OpenSSH using a '--config'
			option that generates per-codespace ssh configuration in OpenSSH format.
			Including this configuration in your ~/.ssh/config improves the user experience
			of tools that integrate with OpenSSH, such as bash/zsh completion of ssh hostnames,
			remote path completion for scp/rsync/sshfs, git ssh remotes, and so on.

			Once that is set up (see the second example below), you can ssh to codespaces as
			if they were ordinary remote hosts (using 'ssh', not 'gh cs ssh').

			Note that the codespace you are connecting to must have an SSH server pre-installed.
			If the docker image being used for the codespace does not have an SSH server,
			install it in your Dockerfile or, for codespaces that use Debian-based images,
			you can add the following to your devcontainer.json:

			"features": {
				"ghcr.io/devcontainers/features/sshd:1": {
					"version": "latest"
				}
			}
		`),
		Example: heredoc.Doc(`
			$ gh codespace ssh

			$ gh codespace ssh --config > ~/.ssh/codespaces
			$ printf 'Match all\nInclude ~/.ssh/codespaces\n' >> ~/.ssh/config
		`),
		PreRunE: func(c *cobra.Command, args []string) error {
			if opts.stdio {
				if opts.selector.codespaceName == "" {
					return errors.New("`--stdio` requires explicit `--codespace`")
				}
				if opts.config {
					return errors.New("cannot use `--stdio` with `--config`")
				}
				if opts.serverPort != 0 {
					return errors.New("cannot use `--stdio` with `--server-port`")
				}
				if opts.profile != "" {
					return errors.New("cannot use `--stdio` with `--profile`")
				}
			}
			if opts.config {
				if opts.profile != "" {
					return errors.New("cannot use `--config` with `--profile`")
				}
				if opts.serverPort != 0 {
					return errors.New("cannot use `--config` with `--server-port`")
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.config {
				return app.printOpenSSHConfig(cmd.Context(), opts)
			} else {
				return app.SSH(cmd.Context(), args, opts)
			}
		},
		DisableFlagsInUseLine: true,
	}

	sshCmd.Flags().StringVarP(&opts.profile, "profile", "", "", "Name of the SSH profile to use")
	sshCmd.Flags().IntVarP(&opts.serverPort, "server-port", "", 0, "SSH server port number (0 => pick unused)")
	opts.selector = AddCodespaceSelector(sshCmd, app.apiClient)
	sshCmd.Flags().BoolVarP(&opts.debug, "debug", "d", false, "Log debug data to a file")
	sshCmd.Flags().StringVarP(&opts.debugFile, "debug-file", "", "", "Path of the file log to")
	sshCmd.Flags().BoolVarP(&opts.config, "config", "", false, "Write OpenSSH configuration to stdout")
	sshCmd.Flags().BoolVar(&opts.stdio, "stdio", false, "Proxy sshd connection to stdio")
	if err := sshCmd.Flags().MarkHidden("stdio"); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	return sshCmd
}

// SSH opens an ssh session or runs an ssh command in a codespace.
func (a *App) SSH(ctx context.Context, sshArgs []string, opts sshOptions) (err error) {
	// Ensure all child tasks (e.g. port forwarding) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	args := sshArgs
	if opts.scpArgs != nil {
		args = opts.scpArgs
	}

	sshContext := ssh.Context{}
	startSSHOptions := rpc.StartSSHServerOptions{}

	keyPair, shouldAddArg, err := selectSSHKeys(ctx, sshContext, args, opts)
	if err != nil {
		return fmt.Errorf("selecting ssh keys: %w", err)
	}

	startSSHOptions.UserPublicKeyFile = keyPair.PublicKeyPath

	if shouldAddArg {
		// For both cp and ssh, flags need to come first in the args (before a command in ssh and files in cp), so prepend this flag
		args = append([]string{"-i", keyPair.PrivateKeyPath}, args...)
	}

	codespace, err := opts.selector.Select(ctx)
	if err != nil {
		return err
	}

	session, err := startLiveShareSession(ctx, codespace, a, opts.debug, opts.debugFile)
	if err != nil {
		return err
	}
	defer safeClose(session, &err)

	var (
		invoker             rpc.Invoker
		remoteSSHServerPort int
		sshUser             string
	)
	err = a.RunWithProgress("Fetching SSH Details", func() (err error) {
		invoker, err = rpc.CreateInvoker(ctx, session)
		if err != nil {
			return
		}

		remoteSSHServerPort, sshUser, err = invoker.StartSSHServerWithOptions(ctx, startSSHOptions)
		return
	})
	if invoker != nil {
		defer safeClose(invoker, &err)
	}
	if err != nil {
		return fmt.Errorf("error getting ssh server details: %w", err)
	}

	if opts.stdio {
		fwd := liveshare.NewPortForwarder(session, "sshd", remoteSSHServerPort, true)
		stdio := liveshare.NewReadWriteHalfCloser(os.Stdin, os.Stdout)
		err := fwd.Forward(ctx, stdio) // always non-nil
		return fmt.Errorf("tunnel closed: %w", err)
	}

	localSSHServerPort := opts.serverPort
	usingCustomPort := localSSHServerPort != 0 // suppress log of command line in Shell

	// Ensure local port is listening before client (Shell) connects.
	// Unless the user specifies a server port, localSSHServerPort is 0
	// and thus the client will pick a random port.
	listen, localSSHServerPort, err := codespaces.ListenTCP(localSSHServerPort)
	if err != nil {
		return err
	}
	defer listen.Close()

	connectDestination := opts.profile
	if connectDestination == "" {
		connectDestination = fmt.Sprintf("%s@localhost", sshUser)
	}

	tunnelClosed := make(chan error, 1)
	go func() {
		fwd := liveshare.NewPortForwarder(session, "sshd", remoteSSHServerPort, true)
		tunnelClosed <- fwd.ForwardToListener(ctx, listen) // always non-nil
	}()

	shellClosed := make(chan error, 1)
	go func() {
		var err error
		if opts.scpArgs != nil {
			// args is the correct variable to use here, we just use scpArgs as the check for which command to run
			err = codespaces.Copy(ctx, args, localSSHServerPort, connectDestination)
		} else {
			err = codespaces.Shell(ctx, a.errLogger, args, localSSHServerPort, connectDestination, usingCustomPort)
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

// selectSSHKeys evaluates available key pairs and select which should be used to connect to the codespace
// using the precedence rules below. If there is no error, a keypair is always returned and additionally a
// bool flag is returned to specify if the private key need be appended to the ssh arguments (it doesn't need
// to be if the key was selected from a -i argument).
//
// Precedence rules:
// 1. Key which is specified by -i
// 2. Automatic key, if it already exists
// 3. First valid keypair in ssh config (according to ssh -G)
// 4. Automatic key, newly created
func selectSSHKeys(
	ctx context.Context,
	sshContext ssh.Context,
	args []string,
	opts sshOptions,
) (*ssh.KeyPair, bool, error) {
	customConfigPath := ""
	for i := 0; i < len(args); i += 1 {
		arg := args[i]

		if arg == "-i" {
			if i+1 >= len(args) {
				return nil, false, errors.New("missing value to -i argument")
			}

			// User manually specified an identity file so just trust it is correct
			return &ssh.KeyPair{
				PrivateKeyPath: args[i+1],
				PublicKeyPath:  args[i+1] + ".pub",
			}, false, nil
		}

		if arg == "-F" && i+1 < len(args) {
			// ssh only pays attention to that last specified -F value, so it's correct to overwrite here
			customConfigPath = args[i+1]
		}
	}

	if autoKeyPair := automaticSSHKeyPair(sshContext); autoKeyPair != nil {
		// If the automatic keys already exist, just use them
		return autoKeyPair, true, nil
	}

	keyPair, err := firstConfiguredKeyPair(ctx, customConfigPath, opts.profile)
	if err != nil {
		if !errors.Is(err, errKeyFileNotFound) {
			return nil, false, fmt.Errorf("checking configured keys: %w", err)
		}

		// no valid key in ssh config, generate one
		keyPair, err = generateAutomaticSSHKeys(sshContext)
		if err != nil {
			return nil, false, fmt.Errorf("generating automatic keypair: %w", err)
		}
	}

	return keyPair, true, nil
}

// automaticSSHKeyPair returns the paths to the automatic key pair files, if they both exist
func automaticSSHKeyPair(sshContext ssh.Context) *ssh.KeyPair {
	publicKeys, err := sshContext.LocalPublicKeys()
	if err != nil {
		// The error would be that the .ssh dir doesn't exist, which just means that the keypair also doesn't exist
		return nil
	}

	for _, publicKey := range publicKeys {
		if filepath.Base(publicKey) != automaticPrivateKeyName+".pub" {
			continue
		}

		privateKey := strings.TrimSuffix(publicKey, ".pub")

		_, err := os.Stat(privateKey)
		if err == nil {
			return &ssh.KeyPair{
				PrivateKeyPath: privateKey,
				PublicKeyPath:  publicKey,
			}
		}
	}

	return nil
}

func generateAutomaticSSHKeys(sshContext ssh.Context) (*ssh.KeyPair, error) {
	keyPair := checkAndUpdateOldKeyPair(sshContext)
	if keyPair != nil {
		return keyPair, nil
	}

	keyPair, err := sshContext.GenerateSSHKey(automaticPrivateKeyName, "")
	if err != nil && !errors.Is(err, ssh.ErrKeyAlreadyExists) {
		return nil, err
	}

	return keyPair, nil
}

// checkAndUpdateOldKeyPair handles backward compatibility with the old keypair names.
// If the old public and private keys both exist they are renamed to the new name.
// The return value is non-nil only if the rename happens.
func checkAndUpdateOldKeyPair(sshContext ssh.Context) *ssh.KeyPair {
	publicKeys, err := sshContext.LocalPublicKeys()
	if err != nil {
		return nil
	}

	for _, publicKey := range publicKeys {
		if filepath.Base(publicKey) != automaticPrivateKeyNameOld+".pub" {
			continue
		}

		privateKey := strings.TrimSuffix(publicKey, ".pub")
		_, err := os.Stat(privateKey)
		if err != nil {
			continue
		}

		// Both old public and private keys exist, rename them to the new name

		sshDir := filepath.Dir(publicKey)

		publicKeyNew := filepath.Join(sshDir, automaticPrivateKeyName+".pub")
		err = os.Rename(publicKey, publicKeyNew)
		if err != nil {
			return nil
		}

		privateKeyNew := filepath.Join(sshDir, automaticPrivateKeyName)
		err = os.Rename(privateKey, privateKeyNew)
		if err != nil {
			return nil
		}

		keyPair := &ssh.KeyPair{
			PublicKeyPath:  publicKeyNew,
			PrivateKeyPath: privateKeyNew,
		}

		return keyPair
	}

	return nil
}

// firstConfiguredKeyPair reads the effective configuration for a localhost
// connection and returns the first valid key pair which would be tried for authentication
func firstConfiguredKeyPair(
	ctx context.Context,
	customConfigFile string,
	customHost string,
) (*ssh.KeyPair, error) {
	sshExe, err := safeexec.LookPath("ssh")
	if err != nil {
		return nil, fmt.Errorf("could not find ssh executable: %w", err)
	}

	// The -G option tells ssh to output the effective config for the given host, but not connect
	sshGArgs := []string{"-G"}

	if customConfigFile != "" {
		sshGArgs = append(sshGArgs, "-F", customConfigFile)
	}

	if customHost != "" {
		sshGArgs = append(sshGArgs, customHost)
	} else {
		sshGArgs = append(sshGArgs, "localhost")
	}

	sshGCmd := exec.CommandContext(ctx, sshExe, sshGArgs...)
	configBytes, err := sshGCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("could not load ssh configuration: %w", err)
	}

	configLines := strings.Split(string(configBytes), "\n")
	for _, line := range configLines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "identityfile ") {
			privateKeyPath := strings.SplitN(line, " ", 2)[1]

			keypair, err := keypairForPrivateKey(privateKeyPath)
			if errors.Is(err, errKeyFileNotFound) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("loading ssh config: %w", err)
			}

			return keypair, nil
		}
	}

	return nil, errKeyFileNotFound
}

// keypairForPrivateKey returns the KeyPair with the specified private key if it and the public key both exist
func keypairForPrivateKey(privateKeyPath string) (*ssh.KeyPair, error) {
	if strings.HasPrefix(privateKeyPath, "~") {
		userHomeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home dir: %w", err)
		}

		// os.Stat can't handle ~, so convert it to the real path
		privateKeyPath = strings.Replace(privateKeyPath, "~", userHomeDir, 1)
	}

	// The default configuration includes standard keys like id_rsa or id_ed25519,
	// but these may not actually exist
	if _, err := os.Stat(privateKeyPath); err != nil {
		return nil, errKeyFileNotFound
	}

	publicKeyPath := privateKeyPath + ".pub"
	if _, err := os.Stat(publicKeyPath); err != nil {
		return nil, errKeyFileNotFound
	}

	return &ssh.KeyPair{
		PrivateKeyPath: privateKeyPath,
		PublicKeyPath:  publicKeyPath,
	}, nil
}

func (a *App) printOpenSSHConfig(ctx context.Context, opts sshOptions) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var csList []*api.Codespace
	if opts.selector.codespaceName == "" {
		err = a.RunWithProgress("Fetching codespaces", func() (err error) {
			csList, err = a.apiClient.ListCodespaces(ctx, api.ListCodespacesOptions{})
			return
		})
	} else {
		var codespace *api.Codespace
		codespace, err = opts.selector.Select(ctx)
		csList = []*api.Codespace{codespace}
	}
	if err != nil {
		return fmt.Errorf("error getting codespace info: %w", err)
	}

	type sshResult struct {
		codespace *api.Codespace
		user      string // on success, the remote ssh username; else nil
		err       error
	}

	sshUsers := make(chan sshResult, len(csList))
	var wg sync.WaitGroup
	var status error
	for _, cs := range csList {
		if cs.State != "Available" && opts.selector.codespaceName == "" {
			fmt.Fprintf(os.Stderr, "skipping unavailable codespace %s: %s\n", cs.Name, cs.State)
			status = cmdutil.SilentError
			continue
		}

		cs := cs
		wg.Add(1)
		go func() {
			result := sshResult{}
			defer wg.Done()

			session, err := codespaces.ConnectToLiveshare(ctx, a, noopLogger(), a.apiClient, cs)
			if err != nil {
				result.err = fmt.Errorf("error connecting to codespace: %w", err)
			} else {
				defer safeClose(session, &err)

				invoker, err := rpc.CreateInvoker(ctx, session)
				if err != nil {
					result.err = fmt.Errorf("error connecting to codespace: %w", err)
				} else {
					defer safeClose(invoker, &err)

					_, result.user, err = invoker.StartSSHServer(ctx)
					if err != nil {
						result.err = fmt.Errorf("error getting ssh server details: %w", err)
					} else {
						result.codespace = cs
					}
				}
			}

			sshUsers <- result
		}()
	}

	go func() {
		wg.Wait()
		close(sshUsers)
	}()

	t, err := template.New("ssh_config").Parse(heredoc.Doc(`
		Host cs.{{.Name}}.{{.EscapedRef}}
			User {{.SSHUser}}
			ProxyCommand {{.GHExec}} cs ssh -c {{.Name}} --stdio -- -i {{.AutomaticIdentityFilePath}}
			UserKnownHostsFile=/dev/null
			StrictHostKeyChecking no
			LogLevel quiet
			ControlMaster auto
			IdentityFile {{.AutomaticIdentityFilePath}}

	`))
	if err != nil {
		return fmt.Errorf("error formatting template: %w", err)
	}

	automaticIdentityFilePath, err := automaticPrivateKeyPath()
	if err != nil {
		return fmt.Errorf("error finding .ssh directory: %w", err)
	}

	ghExec := a.executable.Executable()
	for result := range sshUsers {
		if result.err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", result.err)
			status = cmdutil.SilentError
			continue
		}

		// codespaceSSHConfig contains values needed to write an OpenSSH host
		// configuration for a single codespace. For example:
		//
		// Host {{Name}}.{{EscapedRef}
		//   User {{SSHUser}
		//   ProxyCommand {{GHExec}} cs ssh -c {{Name}} --stdio
		//
		// EscapedRef is included in the name to help distinguish between codespaces
		// when tab-completing ssh hostnames. '/' characters in EscapedRef are
		// flattened to '-' to prevent problems with tab completion or when the
		// hostname appears in ControlMaster socket paths.
		type codespaceSSHConfig struct {
			Name                      string // the codespace name, passed to `ssh -c`
			EscapedRef                string // the currently checked-out branch
			SSHUser                   string // the remote ssh username
			GHExec                    string // path used for invoking the current `gh` binary
			AutomaticIdentityFilePath string // path used for automatic private key `gh cs ssh` would generate
		}

		conf := codespaceSSHConfig{
			Name:                      result.codespace.Name,
			EscapedRef:                strings.ReplaceAll(result.codespace.GitStatus.Ref, "/", "-"),
			SSHUser:                   result.user,
			GHExec:                    ghExec,
			AutomaticIdentityFilePath: automaticIdentityFilePath,
		}
		if err := t.Execute(a.io.Out, conf); err != nil {
			return err
		}
	}

	return status
}

func automaticPrivateKeyPath() (string, error) {
	sshDir, err := config.HomeDirPath(".ssh")
	if err != nil {
		return "", err
	}

	return path.Join(sshDir, automaticPrivateKeyName), nil
}

type cpOptions struct {
	sshOptions
	recursive bool // -r
	expand    bool // -e
}

func newCpCmd(app *App) *cobra.Command {
	var opts cpOptions

	cpCmd := &cobra.Command{
		Use:   "cp [-e] [-r] [-- [<scp flags>...]] <sources>... <dest>",
		Short: "Copy files between local and remote file systems",
		Long: heredoc.Docf(`
			The cp command copies files between the local and remote file systems.

			As with the UNIX %[1]scp%[1]s command, the first argument specifies the source and the last
			specifies the destination; additional sources may be specified after the first,
			if the destination is a directory.

			The %[1]s--recursive%[1]s flag is required if any source is a directory.

			A "remote:" prefix on any file name argument indicates that it refers to
			the file system of the remote (Codespace) machine. It is resolved relative
			to the home directory of the remote user.

			By default, remote file names are interpreted literally. With the %[1]s--expand%[1]s flag,
			each such argument is treated in the manner of %[1]sscp%[1]s, as a Bash expression to
			be evaluated on the remote machine, subject to expansion of tildes, braces, globs,
			environment variables, and backticks. For security, do not use this flag with arguments
			provided by untrusted users; see <https://lwn.net/Articles/835962/> for discussion.
			
			By default, the 'cp' command will create a public/private ssh key pair to authenticate with 
			the codespace inside the ~/.ssh directory.
		`, "`"),
		Example: heredoc.Doc(`
			$ gh codespace cp -e README.md 'remote:/workspaces/$RepositoryName/'
			$ gh codespace cp -e 'remote:~/*.go' ./gofiles/
			$ gh codespace cp -e 'remote:/workspaces/myproj/go.{mod,sum}' ./gofiles/
			$ gh codespace cp -e -- -F ~/.ssh/codespaces_config 'remote:~/*.go' ./gofiles/
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Copy(cmd.Context(), args, opts)
		},
		DisableFlagsInUseLine: true,
	}

	// We don't expose all sshOptions.
	cpCmd.Flags().BoolVarP(&opts.recursive, "recursive", "r", false, "Recursively copy directories")
	cpCmd.Flags().BoolVarP(&opts.expand, "expand", "e", false, "Expand remote file names on remote shell")
	opts.selector = AddCodespaceSelector(cpCmd, app.apiClient)
	cpCmd.Flags().StringVarP(&opts.profile, "profile", "p", "", "Name of the SSH profile to use")
	return cpCmd
}

// Copy copies files between the local and remote file systems.
// The mechanics are similar to 'ssh' but using 'scp'.
func (a *App) Copy(ctx context.Context, args []string, opts cpOptions) error {
	if len(args) < 2 {
		return fmt.Errorf("cp requires source and destination arguments")
	}
	if opts.recursive {
		opts.scpArgs = append(opts.scpArgs, "-r")
	}

	hasRemote := false
	for _, arg := range args {
		if rest := strings.TrimPrefix(arg, "remote:"); rest != arg {
			hasRemote = true
			// scp treats each filename argument as a shell expression,
			// subjecting it to expansion of environment variables, braces,
			// tilde, backticks, globs and so on. Because these present a
			// security risk (see https://lwn.net/Articles/835962/), we
			// disable them by shell-escaping the argument unless the user
			// provided the -e flag.
			if !opts.expand {
				arg = `remote:'` + strings.Replace(rest, `'`, `'\''`, -1) + `'`
			}

		} else if !filepath.IsAbs(arg) {
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
	if !hasRemote {
		return cmdutil.FlagErrorf("at least one argument must have a 'remote:' prefix")
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
		f, err = os.CreateTemp("", "")
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
