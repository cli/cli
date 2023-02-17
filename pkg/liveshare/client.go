// Package liveshare is a Go client library for the Visual Studio Live Share
// service, which provides collaborative, distributed editing and debugging.
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
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/opentracing/opentracing-go"
)

type logger interface {
	Println(v ...interface{})
	Printf(f string, v ...interface{})
}

// An Options specifies Live Share connection parameters.
type Options struct {
	SessionID      string
	SessionToken   string // token for SSH session
	RelaySAS       string
	RelayEndpoint  string
	HostPublicKeys []string
	Logger         logger      // required
	TLSConfig      *tls.Config // (optional)
}

// uri returns a websocket URL for the specified options.
func (opts *Options) uri(action string) (string, error) {
	if opts.SessionID == "" {
		return "", errors.New("SessionID is required")
	}
	if opts.RelaySAS == "" {
		return "", errors.New("RelaySAS is required")
	}
	if opts.RelayEndpoint == "" {
		return "", errors.New("RelayEndpoint is required")
	}

	sas := url.QueryEscape(opts.RelaySAS)
	uri := opts.RelayEndpoint

	if strings.HasPrefix(uri, "http:") {
		uri = strings.Replace(uri, "http:", "ws:", 1)
	} else {
		uri = strings.Replace(uri, "sb:", "wss:", -1)
	}

	uri = strings.Replace(uri, ".net/", ".net:443/$hc/", 1)
	uri = uri + "?sb-hc-action=" + action + "&sb-hc-token=" + sas
	return uri, nil
}

// Connect connects to a Live Share workspace specified by the
// options, and returns a session representing the connection.
// The caller must call the session's Close method to end the session.
func Connect(ctx context.Context, opts Options) (*Session, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "Connect")
	defer span.Finish()

	uri, err := opts.uri("connect")
	if err != nil {
		return nil, err
	}

	sock := newSocket(uri, opts.TLSConfig)
	if err := sock.connect(ctx); err != nil {
		return nil, fmt.Errorf("error connecting websocket: %w", err)
	}

	if opts.SessionToken == "" {
		return nil, errors.New("SessionToken is required")
	}
	ssh := newSSHSession(opts.SessionToken, opts.HostPublicKeys, sock)
	if err := ssh.connect(ctx); err != nil {
		return nil, fmt.Errorf("error connecting to ssh session: %w", err)
	}

	rpc := newRPCClient(ssh)
	rpc.connect(ctx)

	args := joinWorkspaceArgs{
		ID:                      opts.SessionID,
		ConnectionMode:          "local",
		JoiningUserSessionToken: opts.SessionToken,
		ClientCapabilities: clientCapabilities{
			IsNonInteractive: false,
		},
	}
	var result joinWorkspaceResult
	if err := rpc.do(ctx, "workspace.joinWorkspace", &args, &result); err != nil {
		return nil, fmt.Errorf("error joining Live Share workspace: %w", err)
	}

	s := &Session{
		ssh:             ssh,
		rpc:             rpc,
		keepAliveReason: make(chan string, 1),
		logger:          opts.Logger,
	}

	return s, nil
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
