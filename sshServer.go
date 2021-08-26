package liveshare

import (
	"context"
	"errors"
)

type SSHServer struct {
	client *Client
}

func NewSSHServer(client *Client) (*SSHServer, error) {
	if !client.hasJoined() {
		return nil, errors.New("client must join before creating server")
	}
	return &SSHServer{client: client}, nil
}

type SSHServerStartResult struct {
	Result     bool   `json:"result"`
	ServerPort string `json:"serverPort"`
	User       string `json:"user"`
	Message    string `json:"message"`
}

func (s *SSHServer) StartRemoteServer(ctx context.Context) (SSHServerStartResult, error) {
	var response SSHServerStartResult

	if err := s.client.rpc.do(ctx, "ISshServerHostService.startRemoteServer", []string{}, &response); err != nil {
		return response, err
	}

	return response, nil
}
