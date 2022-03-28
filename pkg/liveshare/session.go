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

// registerRequestHandler registers a handler for the given request type with the RPC
// server and returns a callback function to deregister the handler
func (s *Session) registerRequestHandler(requestType string, h handler) func() {
	return s.rpc.register(requestType, h)
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
// Heartbeat ensures it does not send more than one request every "interval" to ratelimit
// how many keepAlives we send at a time.
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
