package codespace

// This file defines the 'gh cs ssh' and 'gh cs cp' subcommands.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/internal/codespaces/grpctunnel"
	"github.com/cli/cli/v2/internal/codespaces/sshconfig"
	"github.com/cli/cli/v2/internal/codespaces/sshkey"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/ssh"
	"github.com/spf13/cobra"
)

type sshOptions struct {
	selector         *CodespaceSelector
	profile          string
	serverPort       int
	printConnDetails bool
	debug            bool
	debugFile        string
	stdio            bool
	config           bool
}

func newSSHCmd(app *App) *cobra.Command {
	var opts sshOptions

	cmd := &cobra.Command{
		Use:   "ssh [<flags>...] [-- <ssh-flags>...] [<command>]",
		Short: "SSH into a codespace",
		Long: heredoc.Docf(`
			The %[1]sssh%[1]s command is used to SSH into a codespace. In its simplest form, you can
			run %[1]sgh cs ssh%[1]s, select a codespace interactively, and connect.

			The %[1]sssh%[1]s command will automatically create a public/private ssh key pair in the
			%[1]s~/.ssh%[1]s directory if you do not have an existing valid key pair. When selecting the
			key pair to use, the preferred order is:

			1. Key specified by %[1]s-i%[1]s in %[1]s<ssh-flags>%[1]s
			2. Automatic key, if it already exists
			3. First valid key pair in ssh config (according to %[1]sssh -G%[1]s)
			4. Automatic key, newly created

			The %[1]sssh%[1]s command also supports deeper integration with OpenSSH using a %[1]s--config%[1]s
			option that generates per-codespace ssh configuration in OpenSSH format.
			Including this configuration in your %[1]s~/.ssh/config%[1]s improves the user experience
			of tools that integrate with OpenSSH, such as Bash/Zsh completion of ssh hostnames,
			remote path completion for %[1]sscp/rsync/sshfs%[1]s, %[1]sgit%[1]s ssh remotes, and so on.

			Once that is set up (see the second example below), you can ssh to codespaces as
			if they were ordinary remote hosts (using %[1]sssh%[1]s, not %[1]sgh cs ssh%[1]s).

			Note that the codespace you are connecting to must have an SSH server pre-installed.
			If the docker image being used for the codespace does not have an SSH server,
			install it in your %[1]sDockerfile%[1]s or, for codespaces that use Debian-based images,
			you can add the following to your %[1]sdevcontainer.json%[1]s:

				"features": {
					"ghcr.io/devcontainers/features/sshd:1": {
						"version": "latest"
					}
				}
		`, "`"),
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
			// Ensure all child tasks (e.g. port forwarding) terminate before return.
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			if cmd.Flag("server-port").Changed {
				opts.printConnDetails = true
			}
			if opts.config {
				return app.printOpenSSHConfig(ctx, opts)
			} else {
				return app.SSH(ctx, args, opts)
			}
		},
		DisableFlagsInUseLine: true,
	}

	cmd.Flags().StringVarP(&opts.profile, "profile", "", "", "Name of the SSH profile to use")
	cmd.Flags().IntVarP(&opts.serverPort, "server-port", "", 0, "SSH server port number (0 => pick unused)")
	opts.selector = AddCodespaceSelector(cmd, app.apiClient)
	cmd.Flags().BoolVarP(&opts.debug, "debug", "d", false, "Log debug data to a file")
	cmd.Flags().StringVarP(&opts.debugFile, "debug-file", "", "", "Path of the file log to")
	cmd.Flags().BoolVarP(&opts.config, "config", "", false, "Write OpenSSH configuration to stdout")
	cmd.Flags().BoolVar(&opts.stdio, "stdio", false, "Proxy sshd connection to stdio")
	if err := cmd.Flags().MarkHidden("stdio"); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	return cmd
}

// SSH opens an ssh session or runs an ssh command in a codespace.
func (a *App) SSH(ctx context.Context, args []string, opts sshOptions) (err error) {
	tunnel, args, err := a.connect(ctx, opts.selector, args, opts.profile)
	if err != nil {
		return err
	}
	defer tunnel.Close()

	if opts.stdio {
		if err := tunnel.ProxyStdio(ctx); err != nil {
			return fmt.Errorf("tunnel closed: %w", err)
		}

		// We always return an error when the tunnel closes.
		return fmt.Errorf("tunnel closed")
	}

	if err := tunnel.Forward(ctx, opts.serverPort); err != nil {
		return fmt.Errorf("error forwarding tunnel: %w", err)
	}

	// Parse the ssh args to determine if the user specified a command
	args, command, err := codespaces.ParseSSHArgs(args)
	if err != nil {
		return err
	}

	// If the user specified a command, we need to keep the shell alive
	// since it will be non-interactive and the codespace might shut down
	// before the command finishes
	if command != nil {
		return tunnel.RunWithKeepAlive(opts.profile, func(destination string, port int) error {
			return codespaces.Shell(
				ctx, a.errLogger, args, command, port, destination, opts.printConnDetails,
			)
		})
	}

	return tunnel.Run(opts.profile, func(destination string, port int) error {
		return codespaces.Shell(
			ctx, a.errLogger, args, command, port, destination, opts.printConnDetails,
		)
	})
}

// connect establishes an SSH connection to a codespace and returns the tunnel and updated args.
// It handles selecting the codespace, creating a tunnel, finding SSH keys, and connecting to the codespace.
func (a *App) connect(ctx context.Context, selector *CodespaceSelector, args []string, profile string) (*grpctunnel.Tunnel, []string, error) {
	codespace, err := selector.Select(ctx)
	if err != nil {
		return nil, nil, err
	}

	tunnel, err := grpctunnel.New(ctx, a, a.apiClient, codespace)
	if err != nil {
		return nil, nil, err
	}

	config := sshkey.NewConfig(ssh.Context{})
	keys, args, err := config.FindKey(ctx, args, profile)
	if err != nil {
		tunnel.Close()
		return nil, nil, fmt.Errorf("selecting ssh keys: %w", err)
	}

	err = a.RunWithProgress("Fetching SSH Details", func() error {
		return tunnel.ConnectWithOptions(ctx, sshkey.ServerOptions(keys))
	})

	if err != nil {
		tunnel.Close()
		return nil, nil, fmt.Errorf("error getting ssh server details: %w", err)
	}

	return tunnel, args, nil
}

func (a *App) printOpenSSHConfig(ctx context.Context, opts sshOptions) (err error) {
	var list []*api.Codespace
	if opts.selector.codespaceName == "" {
		err = a.RunWithProgress("Fetching codespaces", func() (err error) {
			list, err = a.apiClient.ListCodespaces(ctx, api.ListCodespacesOptions{})
			return
		})
	} else {
		var codespace *api.Codespace
		codespace, err = opts.selector.Select(ctx)
		list = []*api.Codespace{codespace}
	}
	if err != nil {
		return fmt.Errorf("error getting codespace info: %w", err)
	}

	type result struct {
		codespace *api.Codespace
		user      string // on success, the remote ssh username; else ""
		err       error
	}

	results := make(chan result, len(list))
	var wg sync.WaitGroup
	var status error
	for idx := range list {
		cs := list[idx]
		if cs.State != "Available" && opts.selector.codespaceName == "" {
			fmt.Fprintf(os.Stderr, "skipping unavailable codespace %s: %s\n", cs.Name, cs.State)
			status = cmdutil.SilentError
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			tunnel, err := grpctunnel.New(ctx, a, a.apiClient, cs)
			if err != nil {
				results <- result{err: err}
				return
			}
			defer safeClose(tunnel, &err)

			if err := tunnel.Connect(ctx); err != nil {
				results <- result{err: fmt.Errorf("error connecting to codespace: %w", err)}
				return
			}

			results <- result{codespace: cs, user: tunnel.User()}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	keys := sshkey.NewConfig(ssh.Context{})
	automaticIdentityFilePath, err := keys.AutomaticPrivateKeyPath()
	if err != nil {
		return fmt.Errorf("error finding .ssh directory: %w", err)
	}

	ghExec := a.executable.Executable()
	for result := range results {
		if result.err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", result.err)
			status = cmdutil.SilentError
			continue
		}

		conf := sshconfig.Config{
			Name:                      result.codespace.Name,
			Ref:                       result.codespace.GitStatus.Ref,
			SSHUser:                   result.user,
			GHExec:                    ghExec,
			AutomaticIdentityFilePath: automaticIdentityFilePath,
		}

		if err := conf.Print(a.io.Out); err != nil {
			return err
		}
	}

	return status
}

type cpOptions struct {
	recursive bool // -r
	expand    bool // -e
	profile   string
	selector  *CodespaceSelector
	scpArgs   []string // the arguments we'll pass to scp once we call it
}

func newCpCmd(app *App) *cobra.Command {
	var opts cpOptions

	cmd := &cobra.Command{
		Use:   "cp [-e] [-r] [-- [<scp flags>...]] <sources>... <dest>",
		Short: "Copy files between local and remote file systems",
		Long: heredoc.Docf(`
			The %[1]scp%[1]s command copies files between the local and remote file systems.

			As with the UNIX %[1]scp%[1]s command, the first argument specifies the source and the last
			specifies the destination; additional sources may be specified after the first,
			if the destination is a directory.

			The %[1]s--recursive%[1]s flag is required if any source is a directory.

			A %[1]sremote:%[1]s prefix on any file name argument indicates that it refers to
			the file system of the remote (Codespace) machine. It is resolved relative
			to the home directory of the remote user.

			By default, remote file names are interpreted literally. With the %[1]s--expand%[1]s flag,
			each such argument is treated in the manner of %[1]sscp%[1]s, as a Bash expression to
			be evaluated on the remote machine, subject to expansion of tildes, braces, globs,
			environment variables, and backticks. For security, do not use this flag with arguments
			provided by untrusted users; see <https://lwn.net/Articles/835962/> for discussion.

			By default, the %[1]scp%[1]s command will create a public/private ssh key pair to authenticate with
			the codespace inside the %[1]s~/.ssh directory%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			$ gh codespace cp -e README.md 'remote:/workspaces/$RepositoryName/'
			$ gh codespace cp -e 'remote:~/*.go' ./gofiles/
			$ gh codespace cp -e 'remote:/workspaces/myproj/go.{mod,sum}' ./gofiles/
			$ gh codespace cp -e -- -F ~/.ssh/codespaces_config 'remote:~/*.go' ./gofiles/
		`),
		PreRunE: func(c *cobra.Command, args []string) error {
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

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Ensure all child tasks (e.g. port forwarding) terminate before return.
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			return app.Copy(ctx, args, opts)
		},
		DisableFlagsInUseLine: true,
	}

	// We don't expose all sshOptions.
	cmd.Flags().BoolVarP(&opts.recursive, "recursive", "r", false, "Recursively copy directories")
	cmd.Flags().BoolVarP(&opts.expand, "expand", "e", false, "Expand remote file names on remote shell")
	opts.selector = AddCodespaceSelector(cmd, app.apiClient)
	cmd.Flags().StringVarP(&opts.profile, "profile", "p", "", "Name of the SSH profile to use")
	return cmd
}

// Copy copies files between the local and remote file systems.
// The mechanics are similar to 'ssh' but using 'scp'.
func (a *App) Copy(ctx context.Context, args []string, opts cpOptions) error {
	tunnel, args, err := a.connect(ctx, opts.selector, args, opts.profile)
	if err != nil {
		return err
	}
	defer tunnel.Close()

	// The cs cp command doesn't have an option to select the server port, we use
	// 0 here so that it pick an unused port ransomly.
	if err := tunnel.Forward(ctx, 0); err != nil {
		return fmt.Errorf("error forwarding tunnel: %w", err)
	}

	return tunnel.Run(opts.profile, func(destination string, port int) error {
		return codespaces.Copy(ctx, args, port, destination)
	})
}
