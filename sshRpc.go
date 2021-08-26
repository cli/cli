package liveshare

import (
	"context"
	"errors"
)

type SshRpc struct {
	client *Client
}

func NewSSHRpc(client *Client) (*SshRpc, error) {
	if !client.hasJoined() {
		return nil, errors.New("client must join before creating server")
	}
	return &SshRpc{client: client}, nil
}

type SshServerStartResult struct {
	Result     bool   `json:"result"`
	ServerPort string `json:"serverPort"`
	User       string `json:"user"`
	Message    string `json:"message"`
}

func (s *SshRpc) StartRemoteServer(ctx context.Context) (SshServerStartResult, error) {
	var response SshServerStartResult

	if err := s.client.rpc.do(ctx, "ISshServerHostService.startRemoteServer", []string{}, &response); err != nil {
		return response, err
	}

	return response, nil
}
