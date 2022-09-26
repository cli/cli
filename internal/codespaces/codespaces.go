package codespaces

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/internal/codespaces/grpc"
	"github.com/cli/cli/v2/pkg/liveshare"
	"golang.org/x/crypto/ssh"
)

const (
	codespacesInternalPort        = 16634
	codespacesInternalSessionName = "CodespacesInternal"
)

func connectionReady(codespace *api.Codespace) bool {
	return codespace.Connection.SessionID != "" &&
		codespace.Connection.SessionToken != "" &&
		codespace.Connection.RelayEndpoint != "" &&
		codespace.Connection.RelaySAS != "" &&
		codespace.State == api.CodespaceStateAvailable
}

//go:generate moq -fmt goimports -rm -skip-ensure -out mock_api.go . apiClient
type ApiClient interface {
	GetCodespace(ctx context.Context, name string, includeConnection bool) (*api.Codespace, error)
	GetOrgMemberCodespace(ctx context.Context, orgName string, userName string, codespaceName string) (*api.Codespace, error)
	ListCodespaces(ctx context.Context, opts api.ListCodespacesOptions) ([]*api.Codespace, error)
	DeleteCodespace(ctx context.Context, name string, orgName string, userName string) error
	StartCodespace(ctx context.Context, name string) error
	StopCodespace(ctx context.Context, name string, orgName string, userName string) error
	CreateCodespace(ctx context.Context, params *api.CreateCodespaceParams) (*api.Codespace, error)
	EditCodespace(ctx context.Context, codespaceName string, params *api.EditCodespaceParams) (*api.Codespace, error)
	GetRepository(ctx context.Context, nwo string) (*api.Repository, error)
	GetCodespacesMachines(ctx context.Context, repoID int, branch, location string) ([]*api.Machine, error)
	GetCodespaceRepositoryContents(ctx context.Context, codespace *api.Codespace, path string) ([]byte, error)
	ListDevContainers(ctx context.Context, repoID int, branch string, limit int) (devcontainers []api.DevContainerEntry, err error)
	GetCodespaceRepoSuggestions(ctx context.Context, partialSearch string, params api.RepoSearchParameters) ([]string, error)
	GetCodespaceBillableOwner(ctx context.Context, nwo string) (*api.User, error)
}

type GrpcClient interface {
	Connect(ctx context.Context, port int, token string) error
	KeepAlive(string)
	GetRunningServer() (int, string, error)
	StartRemoteServer(options grpc.StartSSHServerOptions) (int, string, error)
}

type LiveshareSession interface {
	Close() error
	GetSharedServers(context.Context) ([]*liveshare.Port, error)
	OpenStreamingChannel(context.Context, liveshare.ChannelID) (ssh.Channel, error)
	StartSharing(context.Context, string, int) (liveshare.ChannelID, error)
}

type progressIndicator interface {
	StartProgressIndicatorWithLabel(s string)
	StopProgressIndicator()
}

type logger interface {
	Println(v ...interface{})
	Printf(f string, v ...interface{})
}

// ConnectToLiveshare waits for a Codespace to become running,
// and connects to it using a Live Share session.
func ConnectToLiveshare(ctx context.Context, progress progressIndicator, sessionLogger logger, apiClient ApiClient, codespace *api.Codespace) (sess *liveshare.Session, err error) {
	if codespace.State != api.CodespaceStateAvailable {
		progress.StartProgressIndicatorWithLabel("Starting codespace")
		defer progress.StopProgressIndicator()
		if err := apiClient.StartCodespace(ctx, codespace.Name); err != nil {
			return nil, fmt.Errorf("error starting codespace: %w", err)
		}
	}

	for retries := 0; !connectionReady(codespace); retries++ {
		if retries > 1 {
			time.Sleep(1 * time.Second)
		}

		if retries == 30 {
			return nil, errors.New("timed out while waiting for the codespace to start")
		}

		codespace, err = apiClient.GetCodespace(ctx, codespace.Name, true)
		if err != nil {
			return nil, fmt.Errorf("error getting codespace: %w", err)
		}
	}

	progress.StartProgressIndicatorWithLabel("Connecting to codespace")
	defer progress.StopProgressIndicator()

	return liveshare.Connect(ctx, liveshare.Options{
		ClientName:     "gh",
		SessionID:      codespace.Connection.SessionID,
		SessionToken:   codespace.Connection.SessionToken,
		RelaySAS:       codespace.Connection.RelaySAS,
		RelayEndpoint:  codespace.Connection.RelayEndpoint,
		HostPublicKeys: codespace.Connection.HostPublicKeys,
		Logger:         sessionLogger,
	})
}

// Connects to the gRPC server running on the host VM
func ConnectToGrpcServer(ctx context.Context, grpcClient GrpcClient, token string, session LiveshareSession) error {
	listen, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", 0))
	if err != nil {
		return err
	}

	// Tunnel the remote gRPC server port to the local port
	localGrpcServerPort := listen.Addr().(*net.TCPAddr).Port
	internalTunnelClosed := make(chan error, 1)
	go func() {
		fwd := liveshare.NewPortForwarder(session, grpcClient, codespacesInternalSessionName, codespacesInternalPort, true)
		internalTunnelClosed <- fwd.ForwardToListener(ctx, listen)
	}()

	// Make a connection to the gRPC server
	err = grpcClient.Connect(ctx, localGrpcServerPort, token)

	if err != nil {
		return err
	}

	select {
	case err := <-internalTunnelClosed:
		return fmt.Errorf("internal tunnel closed: %w", err)
	default:
		return nil // success
	}
}
