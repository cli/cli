package liveshare

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/opentracing/opentracing-go"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

// A ChannelID is an identifier for an exposed port on a remote
// container that may be used to open an SSH channel to it.
type ChannelID struct {
	name, condition string
}

// A Session represents the session between a connected Live Share client and server.
type Session struct {
	ssh *sshSession
	rpc *rpcClient

	clientName      string
	keepAliveReason chan string
	logger          logger
}

type StartSSHServerOptions struct {
	UserPublicKeyFile string
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

// StartSSHServer starts an SSH server in the container, installing sshd if necessary, applies specified
// options, and returns the port on which it listens and the user name clients should provide.
func (s *Session) StartSSHServer(ctx context.Context) (int, string, error) {
	return s.StartSSHServerWithOptions(ctx, StartSSHServerOptions{})
}

// StartSSHServerWithOptions starts an SSH server in the container, installing sshd if necessary, applies specified
// options, and returns the port on which it listens and the user name clients should provide.
func (s *Session) StartSSHServerWithOptions(ctx context.Context, options StartSSHServerOptions) (int, string, error) {
	var params struct {
		UserPublicKey string `json:"userPublicKey"`
	}

	var response struct {
		Result     bool   `json:"result"`
		ServerPort string `json:"serverPort"`
		User       string `json:"user"`
		Message    string `json:"message"`
	}

	if options.UserPublicKeyFile != "" {
		publicKeyBytes, err := os.ReadFile(options.UserPublicKeyFile)
		if err != nil {
			return 0, "", fmt.Errorf("failed to read public key file: %w", err)
		}

		params.UserPublicKey = strings.TrimSpace(string(publicKeyBytes))
	}

	if err := s.rpc.do(ctx, "ISshServerHostService.startRemoteServerWithOptions", params, &response); err != nil {
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

// StartJupyterServer starts a Juypyter server in the container and returns
// the port on which it listens and the server URL.
func (s *Session) StartJupyterServer(ctx context.Context) (int, string, error) {
	var response struct {
		Result    bool   `json:"result"`
		Message   string `json:"message"`
		Port      string `json:"port"`
		ServerUrl string `json:"serverUrl"`
	}

	if err := s.rpc.do(ctx, "IJupyterServerHostService.getRunningServer", []string{}, &response); err != nil {
		return 0, "", fmt.Errorf("failed to invoke JupyterLab RPC: %w", err)
	}

	if !response.Result {
		return 0, "", fmt.Errorf("failed to start JupyterLab: %s", response.Message)
	}

	port, err := strconv.Atoi(response.Port)
	if err != nil {
		return 0, "", fmt.Errorf("failed to parse JupyterLab port: %w", err)
	}

	return port, response.ServerUrl, nil
}

// heartbeat runs until context cancellation, periodically checking whether there is a
// reason to keep the connection alive, and if so, notifying the Live Share host to do so.
// Heartbeat ensures it does not send more than one request every "interval" to ratelimit
// how many KeepAlives we send at a time.
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

// KeepAlive accepts a reason that is retained if there is no active reason
// to send to the server.
func (s *Session) KeepAlive(reason string) {
	select {
	case s.keepAliveReason <- reason:
	default:
		// there is already an active keep alive reason
		// so we can ignore this one
	}
}

// StartSharing tells the Live Share host to start sharing the specified port from the container.
// The sessionName describes the purpose of the remote port or service.
// It returns an identifier that can be used to open an SSH channel to the remote port.
func (s *Session) StartSharing(ctx context.Context, sessionName string, port int) (ChannelID, error) {
	args := []interface{}{port, sessionName, fmt.Sprintf("http://localhost:%d", port)}
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		startNotification, err := s.WaitForPortNotification(ctx, port, PortChangeKindStart)
		if err != nil {
			return fmt.Errorf("error while waiting for port notification: %w", err)

		}
		if !startNotification.Success {
			return fmt.Errorf("error while starting port sharing: %s", startNotification.ErrorDetail)
		}
		return nil // success
	})

	var response Port
	g.Go(func() error {
		return s.rpc.do(ctx, "serverSharing.startSharing", args, &response)
	})

	if err := g.Wait(); err != nil {
		return ChannelID{}, err
	}

	return ChannelID{response.StreamName, response.StreamCondition}, nil
}

func (s *Session) OpenStreamingChannel(ctx context.Context, id ChannelID) (ssh.Channel, error) {
	type getStreamArgs struct {
		StreamName string `json:"streamName"`
		Condition  string `json:"condition"`
	}
	args := getStreamArgs{
		StreamName: id.name,
		Condition:  id.condition,
	}
	var streamID string
	if err := s.rpc.do(ctx, "streamManager.getStream", args, &streamID); err != nil {
		return nil, fmt.Errorf("error getting stream id: %w", err)
	}

	span, ctx := opentracing.StartSpanFromContext(ctx, "Session.OpenChannel+SendRequest")
	defer span.Finish()
	_ = ctx // ctx is not currently used

	channel, reqs, err := s.ssh.conn.OpenChannel("session", nil)
	if err != nil {
		return nil, fmt.Errorf("error opening ssh channel for transport: %w", err)
	}
	go ssh.DiscardRequests(reqs)

	requestType := fmt.Sprintf("stream-transport-%s", streamID)
	if _, err = channel.SendRequest(requestType, true, nil); err != nil {
		return nil, fmt.Errorf("error sending channel request: %w", err)
	}

	return channel, nil
}
