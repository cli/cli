package liveshare

import (
	"context"
	"crypto/tls"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// A Client capable of joining a liveshare connection
type Client struct {
	connection Connection
	tlsConfig  *tls.Config

	ssh *sshSession
	rpc *rpcClient
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

// Join is a method that joins the client to the liveshare session
func (c *Client) Join(ctx context.Context) (err error) {
	clientSocket := newSocket(c.connection, c.tlsConfig)
	if err := clientSocket.connect(ctx); err != nil {
		return fmt.Errorf("error connecting websocket: %v", err)
	}

	c.ssh = newSshSession(c.connection.SessionToken, clientSocket)
	if err := c.ssh.connect(ctx); err != nil {
		return fmt.Errorf("error connecting to ssh session: %v", err)
	}

	c.rpc = newRpcClient(c.ssh)
	c.rpc.connect(ctx)

	_, err = c.joinWorkspace(ctx)
	if err != nil {
		return fmt.Errorf("error joining Live Share workspace: %v", err)
	}

	return nil
}

func (c *Client) hasJoined() bool {
	return c.ssh != nil && c.rpc != nil
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

func (c *Client) joinWorkspace(ctx context.Context) (*joinWorkspaceResult, error) {
	args := joinWorkspaceArgs{
		ID:                      c.connection.SessionID,
		ConnectionMode:          "local",
		JoiningUserSessionToken: c.connection.SessionToken,
		ClientCapabilities: clientCapabilities{
			IsNonInteractive: false,
		},
	}

	var result joinWorkspaceResult
	if err := c.rpc.do(ctx, "workspace.joinWorkspace", &args, &result); err != nil {
		return nil, fmt.Errorf("error making workspace.joinWorkspace call: %v", err)
	}

	return &result, nil
}

func (c *Client) openStreamingChannel(ctx context.Context, streamName, condition string) (ssh.Channel, error) {
	args := getStreamArgs{streamName, condition}
	var streamID string
	if err := c.rpc.do(ctx, "streamManager.getStream", args, &streamID); err != nil {
		return nil, fmt.Errorf("error getting stream id: %v", err)
	}

	channel, reqs, err := c.ssh.conn.OpenChannel("session", nil)
	if err != nil {
		return nil, fmt.Errorf("error opening ssh channel for transport: %v", err)
	}
	go ssh.DiscardRequests(reqs)

	requestType := fmt.Sprintf("stream-transport-%s", streamID)
	if _, err = channel.SendRequest(requestType, true, nil); err != nil {
		return nil, fmt.Errorf("error sending channel request: %v", err)
	}

	return channel, nil
}
