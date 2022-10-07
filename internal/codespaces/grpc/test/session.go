package test

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/cli/cli/v2/pkg/liveshare"
	"golang.org/x/crypto/ssh"
)

type Session struct {
}

func (s *Session) KeepAlive(reason string) {
	return
}

func (s *Session) StartSharing(ctx context.Context, sessionName string, port int) (liveshare.ChannelID, error) {
	return liveshare.ChannelID{}, nil
}

// Creates mock SSH channel connected to the mock gRPC server
func (s *Session) OpenStreamingChannel(ctx context.Context, id liveshare.ChannelID) (ssh.Channel, error) {
	dialer := net.Dialer{}
	conn, err := dialer.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", ServerPort))
	if err != nil {
		log.Fatalf("failed to connect to the grpc server: %v", err)
	}
	return &Channel{
		conn: conn,
	}, nil
}
