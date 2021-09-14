package liveshare

import (
	"context"
)

// A SSHServer handles starting the remote SSH server.
// If there is no SSH server available it installs one.
type SSHServer struct {
	session *Session
}

// SSHServer returns a new SSHServer from the LiveShare Session.
func (session *Session) SSHServer() *SSHServer {
	return &SSHServer{session: session}
}

// SSHServerStartResult contains whether or not the start of the SSH server was
// successful. If it succeeded the server port and user is included. If it failed,
// it contains an explanation message.
type SSHServerStartResult struct {
	Result     bool   `json:"result"`
	ServerPort string `json:"serverPort"`
	User       string `json:"user"`
	Message    string `json:"message"`
}

// StartRemoteServer starts or install the remote SSH server and returns the result.
func (s *SSHServer) StartRemoteServer(ctx context.Context) (*SSHServerStartResult, error) {
	var response SSHServerStartResult

	if err := s.session.rpc.do(ctx, "ISshServerHostService.startRemoteServer", []string{}, &response); err != nil {
		return nil, err
	}

	return &response, nil
}
