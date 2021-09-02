package liveshare

import (
	"context"
)

type SSHServer struct {
	session *Session
}

func (session *Session) SSHServer() *SSHServer {
	return &SSHServer{session: session}
}

type SSHServerStartResult struct {
	Result     bool   `json:"result"`
	ServerPort string `json:"serverPort"`
	User       string `json:"user"`
	Message    string `json:"message"`
}

func (s *SSHServer) StartRemoteServer(ctx context.Context) (*SSHServerStartResult, error) {
	var response SSHServerStartResult

	if err := s.session.rpc.do(ctx, "ISshServerHostService.startRemoteServer", []string{}, &response); err != nil {
		return nil, err
	}

	return &response, nil
}
