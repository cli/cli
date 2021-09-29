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
MIIEogIBAAKCAQEAp/Jmzy/HaPNx5Bug09FX5Q/KGY4G9c4DfplhWrn31OQCqNiT
ZSLd46rdXC75liHzE7e5Ic0RJN61cYN9SNArjvEXx2vvs7szhwO7LonwPOvpYpUf
daayrgbr6S46plpx+hEZ1kO/6BqMgFuvnkIVThrEyx5b48ll8zgDABsYrKF8/p1V
SjGfb+bLwjn1NtnZF2prBG5P4ZtMR06HaPglLqBJhmc0ZMG5IZGUE7ew/VrPDqdC
f1v4XvvGiU4BLoKYy4QOhyrCGh9Uk/9u0Ea56M2bh4RqwhbpR8m7TYJZ0DVMLbGW
8C+4lCWp+xRyBNxAQh8qeQVCxYl02hPE4bXLGQIDAQABAoIBAEoVPk6UZ+UexhV2
LnphNOFhFqgxI1bYWmhE5lHsCKuLLLUoW9RYDgL4gw6/1e7o6N3AxFRpre9Soj0B
YIl28k/qf6/DKAhjQnaDKdV8mVF2Swvmdesi7lyfxv6kGtD4wqApXPlMB2IuG94f
E5e+1MEQQ9DJgoU3eNZR1dj9GuRC3PyzPcNNJ2R/MMGFw3sOOVcLOgAukotoicuL
0SiL51rHPQu8a5/darH9EltN1GFeceJSDDhgqMP5T8Tp7g/c3//H6szon4H9W+uN
Z3UrImJ+teJjFOaVDqN93+J2eQSUk0lCPGQCd4U9I4AGDGyU6ucdcLQ58Aha9gmU
uQwkfKUCgYEA0UkuPOSDE9dbXe+yhsbOwMb1kKzJYgFDKjRTSP7D9BOMZu4YyASo
J95R4DWjePlDopafG2tNJoWX+CwUl7Uld1R3Ex6xHBa2B7hwZj860GZtr7D4mdWc
DTVjczAjp4P0K1MIFYQui1mVJterkjKuePiI6q/27L1c2jIa/39BWBcCgYEAzW8R
MFZamVw3eA2JYSpBuqhQgE5gX5IWrmVJZSUhpAQTNG/A4nxf7WGtjy9p99tm0RMb
ld05+sOmNLrzw8Pq8SBpFOd+MAca7lPLS1A2CoaAHbOqRqrzVcZ4EZ2jB3WjoLoq
yctwslGb9KmrhBCdcwT48aPAYUIJCZdqEen2xE8CgYBoMowvywGrvjwCH9X9njvP
5P7cAfrdrY04FQcmP5lmCtmLYZ267/6couaWv33dPBU9fMpIh3rI5BiOebvi8FBw
AgCq50v8lR4Z5+0mKvLoUSbpIy4SwTRJqzwRXHVT8LF/ZH6Q39egj4Bf716/kjYl
im/4kJVatsjk5a9lZ4EsDwKBgERkJ3rKJNtNggHrr8KzSLKVekdc0GTAw+BHRAny
NKLf4Gzij3pXIbBrhlZW2JZ1amNMUzCvN7AuFlUTsDeKL9saiSE2eCIRG3wgVVu7
VmJmqJw6xgNEwkHaEvr6Wd4P4euOTtRjcB9NX/gxzDHpPiGelCoN8+vtCgkxaVSR
aV+tAoGAO4HtLOfBAVDNbVXa27aJAjQSUq8qfkwUNJNz+rwgpVQahfiVkyqAPCQM
IfRJxKWb0Wbt9ojw3AowK/k0d3LZA7FS41JSiiGKIllSGb+i7JKqKW7RHLA3VJ/E
Bq5TLNIbUzPVNVwRcGjUYpOhKU6EIw8phTJOvxnUC+g6MVqBP8U=
-----END RSA PRIVATE KEY-----`

// Server represents a LiveShare relay host server.
type Server struct {
	password       string
	services       map[string]RPCHandleFunc
	relaySAS       string
	streams        map[string]io.ReadWriter
	sshConfig      *ssh.ServerConfig
	httptestServer *httptest.Server
	errCh          chan error
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
	server.httptestServer = httptest.NewTLSServer(http.HandlerFunc(makeConnection(server)))
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
			if req.WantReply {
				if err := req.Reply(true, nil); err != nil {
					sendError(errc, fmt.Errorf("error replying to channel request: %w", err))
					return
				}
			}

			if strings.HasPrefix(req.Type, "stream-transport") {
				go func() {
					if err := forwardStream(ctx, server, req.Type, channel); err != nil {
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

type RPCHandleFunc func(req *jsonrpc2.Request) (interface{}, error)

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

	result, err := handler(req)
	if err != nil {
		sendError(r.server.errCh, fmt.Errorf("error handling: '%s': %w", req.Method, err))
		return
	}

	if err := conn.Reply(ctx, req.ID, result); err != nil {
		sendError(r.server.errCh, fmt.Errorf("error replying: %w", err))
	}
}
