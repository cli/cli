package grpc

// gRPC client implementation to be able to connect to the gRPC server and perform the following operations:
// - Start a remote SSH server
// - Start a remote JupyterLab server
// - Send an activity signal to keep the codespace running when in use

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/grpc/codespace"
	"github.com/cli/cli/v2/internal/codespaces/grpc/jupyter"
	"github.com/cli/cli/v2/internal/codespaces/grpc/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	requestTimeout    = 30 * time.Second
	heartbeatInterval = 1 * time.Minute
	clientName        = "gh"
)

type StartSSHServerOptions struct {
	UserPublicKeyFile string
}

type GRPC struct {
	conn            *grpc.ClientConn
	token           string
	sshClient       ssh.SshServerHostClient
	jupyterClient   jupyter.JupyterServerHostClient
	codespaceClient codespace.CodespaceHostClient
	keepAliveReason chan string
}

func New() *GRPC {
	return &GRPC{}
}

// Connects to the gRPC server on the given port
func (g *GRPC) Connect(ctx context.Context, port int, token string) error {
	// Attempt to connect to the given port
	conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithInsecure(), grpc.WithBlock())

	if err != nil {
		return errors.New(fmt.Sprintf("Failed to connect to the internal server on port %d", port))
	}

	g.conn = conn
	g.token = token
	g.sshClient = ssh.NewSshServerHostClient(conn)
	g.jupyterClient = jupyter.NewJupyterServerHostClient(conn)
	g.codespaceClient = codespace.NewCodespaceHostClient(conn)
	g.keepAliveReason = make(chan string, 1)

	// Send activity heartbeat in the background
	go g.heartbeat(ctx)

	return nil
}

// Appends the authentication token to the gRPC context
func (g *GRPC) appendMetadata(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "Authorization", "Bearer "+g.token)
}

// Starts a remote SSH server to allow the user to connect to the codespace in their local terminal
func (g *GRPC) StartRemoteServer(options StartSSHServerOptions) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	ctx = g.appendMetadata(ctx)
	defer cancel()

	response, err := g.sshClient.StartRemoteServerAsync(ctx, &ssh.StartRemoteServerRequest{UserPublicKey: options.UserPublicKeyFile})

	if err != nil {
		return 0, "", err
	}

	port, err := strconv.Atoi(response.ServerPort)

	return port, response.User, err
}

// Starts a remote JupyterLab server to allow the user to connect to the codespace via JupyterLab in their browser
func (g *GRPC) GetRunningServer() (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	ctx = g.appendMetadata(ctx)
	defer cancel()

	response, err := g.jupyterClient.GetRunningServer(ctx, &jupyter.GetRunningServerRequest{})

	if err != nil {
		return 0, "", err
	}

	port, err := strconv.Atoi(response.Port)

	return port, response.ServerUrl, err
}

// Notifies the codespace that the client is still active
func (g *GRPC) notifyCodespaceOfClientActivity() error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	ctx = g.appendMetadata(ctx)
	defer cancel()

	_, err := g.codespaceClient.NotifyCodespaceOfClientActivity(ctx, &codespace.NotifyCodespaceOfClientActivityRequest{ClientId: clientName, ClientActivities: []string{<-g.keepAliveReason}})

	return err
}

// Sets the keep alive reason for sending to the activity monitor in the codespace
func (s *GRPC) KeepAlive(reason string) {
	select {
	case s.keepAliveReason <- reason:
	default:
		// there is already an active keep alive reason
		// so we can ignore this one
	}
}

// Sends an activity heartbeat to the server to keep the codespace running
func (g *GRPC) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			g.conn.Close()
			return
		case <-ticker.C:
			g.notifyCodespaceOfClientActivity()
		}
	}
}
