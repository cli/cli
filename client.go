// Package liveshare is a Go client library for the Visual Studio Live Share
// service, which provides collaborative, distibuted editing and debugging.
// See https://docs.microsoft.com/en-us/visualstudio/liveshare for an overview.
//
// It provides the ability for a Go program to connect to a Live Share
// workspace (Connect), to expose a TCP port on a remote host
// (UpdateSharedVisibility), to start an SSH server listening on an
// exposed port (StartSSHServer), and to forward connections between
// the remote port and a local listening TCP port (ForwardToListener)
// or a local Go reader/writer (Forward).
package liveshare

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/opentracing/opentracing-go"
	"golang.org/x/crypto/ssh"
)

// A client capable of joining a Live Share workspace.
type client struct {
	connection Connection
	tlsConfig  *tls.Config
}

// An Option updates the initial configuration state of a Live Share connection.
type Option func(*client) error

// WithConnection is a Option that accepts a Connection.
//
// TODO(adonovan): WithConnection is not optional, so it should not be
// not an Option. We should make Connection a mandatory parameter of
// Connect, at which point, why not just merge
// client+Option+Connection, rename it to Options, do away with the
// function mechanism, and express TLS config (etc) as public fields
// of Options with sensible zero values, like websocket.Dialer, etc?
func WithConnection(connection Connection) Option {
	return func(cli *client) error {
		if err := connection.validate(); err != nil {
			return err
		}

		cli.connection = connection
		return nil
	}
}

// WithTLSConfig returns a Connect option that sets the TLS configuration.
func WithTLSConfig(tlsConfig *tls.Config) Option {
	return func(cli *client) error {
		cli.tlsConfig = tlsConfig
		return nil
	}
}

// Connect connects to a Live Share workspace specified by the
// options, and returns a session representing the connection.
// The caller must call the session's Close method to end the session.
func Connect(ctx context.Context, opts ...Option) (*Session, error) {
	cli := new(client)
	for _, opt := range opts {
		if err := opt(cli); err != nil {
			return nil, fmt.Errorf("error applying Live Share connect option: %w", err)
		}
	}

	span, ctx := opentracing.StartSpanFromContext(ctx, "Connect")
	defer span.Finish()

	sock := newSocket(cli.connection, cli.tlsConfig)
	if err := sock.connect(ctx); err != nil {
		return nil, fmt.Errorf("error connecting websocket: %w", err)
	}

	ssh := newSSHSession(cli.connection.SessionToken, sock)
	if err := ssh.connect(ctx); err != nil {
		return nil, fmt.Errorf("error connecting to ssh session: %w", err)
	}

	rpc := newRPCClient(ssh)
	rpc.connect(ctx)

	args := joinWorkspaceArgs{
		ID:                      cli.connection.SessionID,
		ConnectionMode:          "local",
		JoiningUserSessionToken: cli.connection.SessionToken,
		ClientCapabilities: clientCapabilities{
			IsNonInteractive: false,
		},
	}
	var result joinWorkspaceResult
	if err := rpc.do(ctx, "workspace.joinWorkspace", &args, &result); err != nil {
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
