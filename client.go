package liveshare

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/opentracing/opentracing-go"
	"golang.org/x/crypto/ssh"
)

// A Client capable of joining a Live Share workspace.
type Client struct {
	connection Connection
	tlsConfig  *tls.Config
}

// A ClientOption is a function that modifies a client
type ClientOption func(*Client) error

// NewClient accepts a range of options, applies them and returns a client
func NewClient(opts ...ClientOption) (*Client, error) {
	client := new(Client)

	for _, o := range opts {
		if err := o(client); err != nil {
			return nil, err
		}
	}

	return client, nil
}

// WithConnection is a ClientOption that accepts a Connection
func WithConnection(connection Connection) ClientOption {
	return func(c *Client) error {
		if err := connection.validate(); err != nil {
			return err
		}

		c.connection = connection
		return nil
	}
}

func WithTLSConfig(tlsConfig *tls.Config) ClientOption {
	return func(c *Client) error {
		c.tlsConfig = tlsConfig
		return nil
	}
}

// JoinWorkspace connects the client to the server's Live Share
// workspace and returns a session representing their connection.
func (c *Client) JoinWorkspace(ctx context.Context) (*Session, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "Client.JoinWorkspace")
	defer span.Finish()

	clientSocket := newSocket(c.connection, c.tlsConfig)
	if err := clientSocket.connect(ctx); err != nil {
		return nil, fmt.Errorf("error connecting websocket: %w", err)
	}

	ssh := newSSHSession(c.connection.SessionToken, clientSocket)
	if err := ssh.connect(ctx); err != nil {
		return nil, fmt.Errorf("error connecting to ssh session: %w", err)
	}

	rpc := newRPCClient(ssh)
	rpc.connect(ctx)
	if _, err := c.joinWorkspace(ctx, rpc); err != nil {
		return nil, fmt.Errorf("error joining Live Share workspace: %w", err)
	}

	return &Session{ssh: ssh, rpc: rpc}, nil
}

type clientCapabilities struct {
	IsNonInteractive bool `json:"isNonInteractive"`
}

type joinWorkspaceArgs struct {
	ID                      string             `json:"id"`
	ConnectionMode          string             `json:"connectionMode"`
	JoiningUserSessionToken string             `json:"joiningUserSessionToken"`
	ClientCapabilities      clientCapabilities `json:"clientCapabilities"`
}

type joinWorkspaceResult struct {
	SessionNumber int `json:"sessionNumber"`
}

// A channelID is an identifier for an exposed port on a remote
// container that may be used to open an SSH channel to it.
type channelID struct {
	name, condition string
}

func (c *Client) joinWorkspace(ctx context.Context, rpc *rpcClient) (*joinWorkspaceResult, error) {
	args := joinWorkspaceArgs{
		ID:                      c.connection.SessionID,
		ConnectionMode:          "local",
		JoiningUserSessionToken: c.connection.SessionToken,
		ClientCapabilities: clientCapabilities{
			IsNonInteractive: false,
		},
	}

	var result joinWorkspaceResult
	if err := rpc.do(ctx, "workspace.joinWorkspace", &args, &result); err != nil {
		return nil, fmt.Errorf("error making workspace.joinWorkspace call: %w", err)
	}

	return &result, nil
}

func (s *Session) openStreamingChannel(ctx context.Context, id channelID) (ssh.Channel, error) {
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
