package liveshare

import (
	"context"
	"fmt"

	"github.com/opentracing/opentracing-go"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

// A ChannelID is an identifier for an exposed port on a remote
// container that may be used to open an SSH channel to it.
type ChannelID struct {
	name, condition string
}

// Interface to allow the mocking of the liveshare session
type LiveshareSession interface {
	Close() error
	GetSharedServers(context.Context) ([]*Port, error)
	KeepAlive(string)
	OpenStreamingChannel(context.Context, ChannelID) (ssh.Channel, error)
	StartSharing(context.Context, string, int) (ChannelID, error)
	GetKeepAliveReason() string
}

// A Session represents the session between a connected Live Share client and server.
type Session struct {
	ssh *sshSession
	rpc *rpcClient

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

// Fetches the keep alive reason from the channel and returns it.
func (s *Session) GetKeepAliveReason() string {
	return <-s.keepAliveReason
}

// registerRequestHandler registers a handler for the given request type with the RPC
// server and returns a callback function to deregister the handler
func (s *Session) registerRequestHandler(requestType string, h handler) func() {
	return s.rpc.register(requestType, h)
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
