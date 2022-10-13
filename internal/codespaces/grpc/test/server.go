package test

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/cli/cli/v2/internal/codespaces/grpc/jupyter"
	"google.golang.org/grpc"
)

const (
	ServerPort = 50051
)

var (
	JupyterPort      = 1234
	JupyterServerUrl = "http://localhost:1234?token=1234"
	JupyterMessage   = ""
	JupyterResult    = true
)

type server struct {
	jupyter.UnimplementedJupyterServerHostServer
}

func (s *server) GetRunningServer(ctx context.Context, in *jupyter.GetRunningServerRequest) (*jupyter.GetRunningServerResponse, error) {
	return &jupyter.GetRunningServerResponse{
		Port:      strconv.Itoa(JupyterPort),
		ServerUrl: JupyterServerUrl,
		Message:   JupyterMessage,
		Result:    JupyterResult,
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
