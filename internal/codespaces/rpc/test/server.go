package test

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/cli/cli/v2/internal/codespaces/rpc/codespace"
	"github.com/cli/cli/v2/internal/codespaces/rpc/jupyter"
	"google.golang.org/grpc"
)

const (
	ServerPort = 50051
)

// Mock responses for the `GetRunningServer` RPC method
var (
	JupyterPort      = 1234
	JupyterServerUrl = "http://localhost:1234?token=1234"
	JupyterMessage   = ""
	JupyterResult    = true
)

// Mock responses for the `RebuildContainerAsync` RPC method
var (
	RebuildContainer = true
)

type server struct {
	jupyter.UnimplementedJupyterServerHostServer
	codespace.CodespaceHostServer
}

func (s *server) GetRunningServer(ctx context.Context, in *jupyter.GetRunningServerRequest) (*jupyter.GetRunningServerResponse, error) {
	return &jupyter.GetRunningServerResponse{
		Port:      strconv.Itoa(JupyterPort),
		ServerUrl: JupyterServerUrl,
		Message:   JupyterMessage,
		Result:    JupyterResult,
	}, nil
}

func (s *server) RebuildContainerAsync(ctx context.Context, in *codespace.RebuildContainerRequest) (*codespace.RebuildContainerResponse, error) {
	return &codespace.RebuildContainerResponse{
		RebuildContainer: RebuildContainer,
	}, nil
}

// Starts the mock gRPC server listening on port 50051
func StartServer(ctx context.Context) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", ServerPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	defer listener.Close()

	s := grpc.NewServer()
	jupyter.RegisterJupyterServerHostServer(s, &server{})
	codespace.RegisterCodespaceHostServer(s, &server{})

	ch := make(chan error, 1)
	go func() {
		if err := s.Serve(listener); err != nil {
			ch <- fmt.Errorf("failed to serve: %v", err)
		}
	}()

	select {
	case <-ctx.Done():
		s.Stop()
		return ctx.Err()
	case err := <-ch:
		return err
	}
}
