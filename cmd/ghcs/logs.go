package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/github/ghcs/api"
	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/codespaces"
	"github.com/github/go-liveshare"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func newLogsCmd() *cobra.Command {
	var (
		codespace string
		tail      bool
		follow    bool
	)

	log := output.NewLogger(os.Stdout, os.Stderr, false)

	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Access codespace logs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				log.Errorln("<codespace> argument is deprecated. Use --codespace instead.")
				codespace = args[0]
			}
			if tail {
				log.Errorln("--tail flag is deprecated. Use --follow instead.")
				follow = true
			}
			return logs(context.Background(), log, codespace, follow)
		},
	}

	logsCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
	logsCmd.Flags().BoolVarP(&tail, "tail", "t", false, "Tail the logs (deprecated, use --follow)")
	logsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "Tail and follow the logs")

	return logsCmd
}

func init() {
	rootCmd.AddCommand(newLogsCmd())
}

func logs(ctx context.Context, log *output.Logger, codespaceName string, follow bool) error {
	// Ensure all child tasks (port forwarding, remote exec) terminate before return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("getting user: %v", err)
	}

	codespace, token, err := codespaces.GetOrChooseCodespace(ctx, apiClient, user, codespaceName)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %v", err)
	}

	session, err := codespaces.ConnectToLiveshare(ctx, log, apiClient, user.Login, token, codespace)
	if err != nil {
		return fmt.Errorf("connecting to Live Share: %v", err)
	}

	// Ensure local port is listening before client (getPostCreateOutput) connects.
	listen, err := net.Listen("tcp", ":0") // arbitrary port
	if err != nil {
		return err
	}
	defer listen.Close()
	localPort := listen.Addr().(*net.TCPAddr).Port

	remoteSSHServerPort, sshUser, err := codespaces.StartSSHServer(ctx, session, log)
	if err != nil {
		return fmt.Errorf("error getting ssh server details: %v", err)
	}

	cmdType := "cat"
	if follow {
		cmdType = "tail -f"
	}

	dst := fmt.Sprintf("%s@localhost", sshUser)
	cmd := codespaces.NewRemoteCommand(
		ctx, localPort, dst, fmt.Sprintf("%s /workspaces/.codespaces/.persistedshare/creation.log", cmdType),
	)

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		fwd := liveshare.NewPortForwarder(session, "sshd", remoteSSHServerPort)
		err := fwd.ForwardToListener(ctx, listen) // error is non-nil
		return fmt.Errorf("connection closed: %v", err)
	})
	group.Go(cmd.Run)
	return group.Wait()
}
