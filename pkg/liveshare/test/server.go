package livesharetest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"
	"golang.org/x/crypto/ssh"
)

const sshPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIICXgIBAAKBgQC6VU6XsMaTot9ogsGcJ+juvJOmDvvCZmgJRTRwKkW0u2BLz4yV
rCzQcxaY4kaIuR80Y+1f0BLnZgh4pTREDR0T+p8hUsDSHim1ttKI8rK0hRtJ2qhY
lR4qt7P51rPA4KFA9z9gDjTwQLbDq21QMC4+n4d8CL3xRVGtlUAMM3Kl3wIDAQAB
AoGBAI8UemkYoSM06gBCh5D1RHQt8eKNltzL7g9QSNfoXeZOC7+q+/TiZPcbqLp0
5lyOalu8b8Ym7J0rSE377Ypj13LyHMXS63e4wMiXv3qOl3GDhMLpypnJ8PwqR2b8
IijL2jrpQfLu6IYqlteA+7e9aEexJa1RRwxYIyq6pG1IYpbhAkEA9nKgtj3Z6ZDC
46IdqYzuUM9ZQdcw4AFr407+lub7tbWe5pYmaq3cT725IwLw081OAmnWJYFDMa/n
IPl9YcZSPQJBAMGOMbPs/YPkQAsgNdIUlFtK3o41OrrwJuTRTvv0DsbqDV0LKOiC
t8oAQQvjisH6Ew5OOhFyIFXtvZfzQMJppksCQQDWFd+cUICTUEise/Duj9maY3Uz
J99ySGnTbZTlu8PfJuXhg3/d3ihrMPG6A1z3cPqaSBxaOj8H07mhQHn1zNU1AkEA
hkl+SGPrO793g4CUdq2ahIA8SpO5rIsDoQtq7jlUq0MlhGFCv5Y5pydn+bSjx5MV
933kocf5kUSBntPBIWElYwJAZTm5ghu0JtSE6t3km0iuj7NGAQSdb6mD8+O7C3CP
FU3vi+4HlBysaT6IZ/HG+/dBsr4gYp4LGuS7DbaLuYw/uw==
-----END RSA PRIVATE KEY-----`

const SSHPublicKey = `AAAAB3NzaC1yc2EAAAADAQABAAAAgQC6VU6XsMaTot9ogsGcJ+juvJOmDvvCZmgJRTRwKkW0u2BLz4yVrCzQcxaY4kaIuR80Y+1f0BLnZgh4pTREDR0T+p8hUsDSHim1ttKI8rK0hRtJ2qhYlR4qt7P51rPA4KFA9z9gDjTwQLbDq21QMC4+n4d8CL3xRVGtlUAMM3Kl3w==`

// Server represents a LiveShare relay host server.
type Server struct {
	password       string
	services       map[string]RPCHandleFunc
	relaySAS       string
	streams        map[string]io.ReadWriter
	sshConfig      *ssh.ServerConfig
	httptestServer *httptest.Server
	errCh          chan error
	nonSecure      bool
}

// NewServer creates a new Server. ServerOptions can be passed to configure
// the SSH password, backing service, secrets and more.
func NewServer(opts ...ServerOption) (*Server, error) {
	server := new(Server)

	for _, o := range opts {
		if err := o(server); err != nil {
			return nil, err
		}
	}

	server.sshConfig = &ssh.ServerConfig{
		PasswordCallback: sshPasswordCallback(server.password),
	}
	privateKey, err := ssh.ParsePrivateKey([]byte(sshPrivateKey))
	if err != nil {
		return nil, fmt.Errorf("error parsing key: %w", err)
	}
	server.sshConfig.AddHostKey(privateKey)

	server.errCh = make(chan error, 1)

	if server.nonSecure {
		server.httptestServer = httptest.NewServer(http.HandlerFunc(makeConnection(server)))
	} else {
		server.httptestServer = httptest.NewTLSServer(http.HandlerFunc(makeConnection(server)))
	}
	return server, nil
}

// ServerOption is used to configure the Server.
type ServerOption func(*Server) error

// WithPassword configures the Server password for SSH.
func WithPassword(password string) ServerOption {
	return func(s *Server) error {
		s.password = password
		return nil
	}
}

// WithNonSecure configures the Server as non-secure.
func WithNonSecure() ServerOption {
	return func(s *Server) error {
		s.nonSecure = true
		return nil
	}
}

// WithService accepts a mock RPC service for the Server to invoke.
func WithService(serviceName string, handler RPCHandleFunc) ServerOption {
	return func(s *Server) error {
		if s.services == nil {
			s.services = make(map[string]RPCHandleFunc)
		}

		s.services[serviceName] = handler
		return nil
	}
}

// WithRelaySAS configures the relay SAS configuration key.
func WithRelaySAS(sas string) ServerOption {
	return func(s *Server) error {
		s.relaySAS = sas
		return nil
	}
}

// WithStream allows you to specify a mock data stream for the server.
func WithStream(name string, stream io.ReadWriter) ServerOption {
	return func(s *Server) error {
		if s.streams == nil {
			s.streams = make(map[string]io.ReadWriter)
		}
		s.streams[name] = stream
		return nil
	}
}

func sshPasswordCallback(serverPassword string) func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
	return func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if string(password) == serverPassword {
			return nil, nil
		}
		return nil, errors.New("password rejected")
	}
}

// Close closes the underlying httptest Server.
func (s *Server) Close() {
	s.httptestServer.Close()
}

// URL returns the httptest Server url.
func (s *Server) URL() string {
	return s.httptestServer.URL
}

func (s *Server) Err() <-chan error {
	return s.errCh
}

var upgrader = websocket.Upgrader{}

func makeConnection(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if server.relaySAS != "" {
			// validate the sas key
			sasParam := req.URL.Query().Get("sb-hc-token")
			if sasParam != server.relaySAS {
				sendError(server.errCh, errors.New("error validating sas"))
				return
			}
		}
		c, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			sendError(server.errCh, fmt.Errorf("error upgrading connection: %w", err))
			return
		}
		defer func() {
			if err := c.Close(); err != nil {
				sendError(server.errCh, err)
			}
		}()

		socketConn := newSocketConn(c)
		_, chans, reqs, err := ssh.NewServerConn(socketConn, server.sshConfig)
		if err != nil {
			sendError(server.errCh, fmt.Errorf("error creating new ssh conn: %w", err))
			return
		}
		go ssh.DiscardRequests(reqs)

		if err := handleChannels(ctx, server, chans); err != nil {
			sendError(server.errCh, err)
		}
	}
}

// sendError does a non-blocking send of the error to the err channel.
func sendError(errc chan<- error, err error) {
	select {
	case errc <- err:
	default:
		// channel is blocked with a previous error, so we ignore
		// this current error
	}
}

// awaitError waits for the context to finish and returns its error (if any).
// It also waits for an err to come through the err channel.
func awaitError(ctx context.Context, errc <-chan error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errc:
		return err
	}
}

// handleChannels services the sshChannels channel. For each SSH channel received
// it creates a go routine to service the channel's requests. It returns on the first
// error encountered.
func handleChannels(ctx context.Context, server *Server, sshChannels <-chan ssh.NewChannel) error {
	errc := make(chan error, 1)
	go func() {
		for sshCh := range sshChannels {
			ch, reqs, err := sshCh.Accept()
			if err != nil {
				sendError(errc, fmt.Errorf("failed to accept channel: %w", err))
				return
			}

			go func() {
				if err := handleRequests(ctx, server, ch, reqs); err != nil {
					sendError(errc, fmt.Errorf("failed to handle requests: %w", err))
				}
			}()

			handleChannel(server, ch)
		}
	}()
	return awaitError(ctx, errc)
}

// handleRequests services the SSH channel requests channel. It replies to requests and
// when stream transport requests are encountered, creates a go routine to create a
// bi-directional data stream between the channel and server stream. It returns on the first error
// encountered.
func handleRequests(ctx context.Context, server *Server, channel ssh.Channel, reqs <-chan *ssh.Request) error {
	errc := make(chan error, 1)
	go func() {
		for req := range reqs {
			r := req
			if r.WantReply {
				if err := r.Reply(true, nil); err != nil {
					sendError(errc, fmt.Errorf("error replying to channel request: %w", err))
					return
				}
			}

			if strings.HasPrefix(r.Type, "stream-transport") {
				go func() {
					if err := forwardStream(ctx, server, r.Type, channel); err != nil {
						sendError(errc, fmt.Errorf("failed to forward stream: %w", err))
					}
				}()
			}
		}
	}()

	return awaitError(ctx, errc)
}

// concurrentStream is a concurrency safe io.ReadWriter.
type concurrentStream struct {
	sync.RWMutex
	stream io.ReadWriter
}

func newConcurrentStream(rw io.ReadWriter) *concurrentStream {
	return &concurrentStream{stream: rw}
}

func (cs *concurrentStream) Read(b []byte) (int, error) {
	cs.RLock()
	defer cs.RUnlock()
	return cs.stream.Read(b)
}

func (cs *concurrentStream) Write(b []byte) (int, error) {
	cs.Lock()
	defer cs.Unlock()
	return cs.stream.Write(b)
}

// forwardStream does a bi-directional copy of the stream <-> with the SSH channel. The io.Copy
// runs until an error is encountered.
func forwardStream(ctx context.Context, server *Server, streamName string, channel ssh.Channel) (err error) {
	simpleStreamName := strings.TrimPrefix(streamName, "stream-transport-")
	stream, found := server.streams[simpleStreamName]
	if !found {
		return fmt.Errorf("stream '%s' not found", simpleStreamName)
	}
	defer func() {
		if closeErr := channel.Close(); err == nil && closeErr != io.EOF {
			err = closeErr
		}
	}()

	errc := make(chan error, 2)
	copy := func(dst io.Writer, src io.Reader) {
		if _, err := io.Copy(dst, src); err != nil {
			errc <- err
		}
	}

	csStream := newConcurrentStream(stream)
	go copy(csStream, channel)
	go copy(channel, csStream)

	return awaitError(ctx, errc)
}

func handleChannel(server *Server, channel ssh.Channel) {
	stream := jsonrpc2.NewBufferedStream(channel, jsonrpc2.VSCodeObjectCodec{})
	jsonrpc2.NewConn(context.Background(), stream, newRPCHandler(server))
}

type RPCHandleFunc func(conn *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error)

type rpcHandler struct {
	server *Server
}

func newRPCHandler(server *Server) *rpcHandler {
	return &rpcHandler{server}
}

// Handle satisfies the jsonrpc2 pkg handler interface. It tries to find a mocked
// RPC service method and if found, it invokes the handler and replies to the request.
func (r *rpcHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	handler, found := r.server.services[req.Method]
	if !found {
		sendError(r.server.errCh, fmt.Errorf("RPC Method: '%s' not serviced", req.Method))
		return
	}

	result, err := handler(conn, req)
	if err != nil {
		sendError(r.server.errCh, fmt.Errorf("error handling: '%s': %w", req.Method, err))
		return
	}

	if err := conn.Reply(ctx, req.ID, result); err != nil {
		sendError(r.server.errCh, fmt.Errorf("error replying: %w", err))
	}
}
