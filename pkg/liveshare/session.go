package liveshare

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// A Session represents the session between a connected Live Share client and server.
type Session struct {
	ssh *sshSession
	rpc *rpcClient

	clientName      string
	keepAliveReason chan string
	logger          logger
}

// Close should be called by users to clean up RPC and SSH resources whenever the session
// is no longer active.
func (s *Session) Close() error {
	// Closing the RPC conn closes the underlying stream (SSH)
	// So we only need to close once
	if err := s.rpc.Close(); err != nil {
		s.ssh.Close() // close SSH and ignore error
		return fmt.Errorf("error while closing Live Share session: %w", err)
	}

	return nil
}

// Port describes a port exposed by the container.
type Port struct {
	SourcePort                       int    `json:"sourcePort"`
	DestinationPort                  int    `json:"destinationPort"`
	SessionName                      string `json:"sessionName"`
	StreamName                       string `json:"streamName"`
	StreamCondition                  string `json:"streamCondition"`
	BrowseURL                        string `json:"browseUrl"`
	IsPublic                         bool   `json:"isPublic"`
	IsTCPServerConnectionEstablished bool   `json:"isTCPServerConnectionEstablished"`
	HasTLSHandshakePassed            bool   `json:"hasTLSHandshakePassed"`
}

// startSharing tells the Live Share host to start sharing the specified port from the container.
// The sessionName describes the purpose of the remote port or service.
// It returns an identifier that can be used to open an SSH channel to the remote port.
func (s *Session) startSharing(ctx context.Context, sessionName string, port int) (channelID, error) {
	args := []interface{}{port, sessionName, fmt.Sprintf("http://localhost:%d", port)}
	var response Port
	if err := s.rpc.do(ctx, "serverSharing.startSharing", args, &response); err != nil {
		return channelID{}, err
	}

	return channelID{response.StreamName, response.StreamCondition}, nil
}

// GetSharedServers returns a description of each container port
// shared by a prior call to StartSharing by some client.
func (s *Session) GetSharedServers(ctx context.Context) ([]*Port, error) {
	var response []*Port
	if err := s.rpc.do(ctx, "serverSharing.getSharedServers", []string{}, &response); err != nil {
		return nil, err
	}

	return response, nil
}

// UpdateSharedVisibility controls port permissions and whether it can be accessed publicly
// via the Browse URL
func (s *Session) UpdateSharedVisibility(ctx context.Context, port int, public bool) error {
	if err := s.rpc.do(ctx, "serverSharing.updateSharedServerVisibility", []interface{}{port, public}, nil); err != nil {
		return err
	}

	return nil
}

// StartsSSHServer starts an SSH server in the container, installing sshd if necessary,
// and returns the port on which it listens and the user name clients should provide.
func (s *Session) StartSSHServer(ctx context.Context) (int, string, error) {
	var response struct {
		Result     bool   `json:"result"`
		ServerPort string `json:"serverPort"`
		User       string `json:"user"`
		Message    string `json:"message"`
	}

	if err := s.rpc.do(ctx, "ISshServerHostService.startRemoteServer", []string{}, &response); err != nil {
		return 0, "", err
	}

	if !response.Result {
		return 0, "", fmt.Errorf("failed to start server: %s", response.Message)
	}

	port, err := strconv.Atoi(response.ServerPort)
	if err != nil {
		return 0, "", fmt.Errorf("failed to parse port: %w", err)
	}

	return port, response.User, nil
}

// heartbeat runs until context cancellation, periodically checking whether there is a
// reason to keep the connection alive, and if so, notifying the Live Share host to do so.
func (s *Session) heartbeat(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.logger.Println("Heartbeat tick")
			reason := <-s.keepAliveReason
			s.logger.Println("Keep alive reason: " + reason)
			if err := s.notifyHostOfActivity(ctx, reason); err != nil {
				s.logger.Printf("Failed to notify host of activity: %s\n", err)
			}
		}
	}
}

// notifyHostOfActivity notifies the Live Share host of client activity.
func (s *Session) notifyHostOfActivity(ctx context.Context, activity string) error {
	activities := []string{activity}
	params := []interface{}{s.clientName, activities}
	return s.rpc.do(ctx, "ICodespaceHostService.notifyCodespaceOfClientActivity", params, nil)
}

// keepAlive accepts a reason that is retained if there is no active reason
// to send to the server.
func (s *Session) keepAlive(reason string) {
	select {
	case s.keepAliveReason <- reason:
	default:
		// there is already an active keep alive reason
		// so we can ignore this one
	}
}
