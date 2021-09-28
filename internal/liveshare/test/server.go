package livesharetest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

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

type Server struct {
	password string
	services map[string]RPCHandleFunc
	relaySAS string
	streams  map[string]io.ReadWriter

	sshConfig      *ssh.ServerConfig
	httptestServer *httptest.Server
	errCh          chan error
}

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

	server.errCh = make(chan error)
	server.httptestServer = httptest.NewTLSServer(http.HandlerFunc(makeConnection(server)))
	return server, nil
}

type ServerOption func(*Server) error

func WithPassword(password string) ServerOption {
	return func(s *Server) error {
		s.password = password
		return nil
	}
}

func WithService(serviceName string, handler RPCHandleFunc) ServerOption {
	return func(s *Server) error {
		if s.services == nil {
			s.services = make(map[string]RPCHandleFunc)
		}

		s.services[serviceName] = handler
		return nil
	}
}

func WithRelaySAS(sas string) ServerOption {
	return func(s *Server) error {
		s.relaySAS = sas
		return nil
	}
}

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

func (s *Server) Close() {
	s.httptestServer.Close()
}

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
				server.errCh <- errors.New("error validating sas")
				return
			}
		}
		c, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			server.errCh <- fmt.Errorf("error upgrading connection: %w", err)
			return
		}
		defer c.Close()

		socketConn := newSocketConn(c)
		_, chans, reqs, err := ssh.NewServerConn(socketConn, server.sshConfig)
		if err != nil {
			server.errCh <- fmt.Errorf("error creating new ssh conn: %w", err)
			return
		}
		go ssh.DiscardRequests(reqs)

		for newChannel := range chans {
			ch, reqs, err := newChannel.Accept()
			if err != nil {
				server.errCh <- fmt.Errorf("error accepting new channel: %w", err)
				return
			}
			go handleNewRequests(ctx, server, ch, reqs)
			go handleNewChannel(server, ch)
		}
	}
}

func handleNewRequests(ctx context.Context, server *Server, channel ssh.Channel, reqs <-chan *ssh.Request) {
	for req := range reqs {
		if req.WantReply {
			if err := req.Reply(true, nil); err != nil {
				server.errCh <- fmt.Errorf("error replying to channel request: %w", err)
			}
		}
		if strings.HasPrefix(req.Type, "stream-transport") {
			forwardStream(ctx, server, req.Type, channel)
		}
	}
}

func forwardStream(ctx context.Context, server *Server, streamName string, channel ssh.Channel) {
	simpleStreamName := strings.TrimPrefix(streamName, "stream-transport-")
	stream, found := server.streams[simpleStreamName]
	if !found {
		server.errCh <- fmt.Errorf("stream '%s' not found", simpleStreamName)
		return
	}

	copy := func(dst io.Writer, src io.Reader) {
		if _, err := io.Copy(dst, src); err != nil {
			fmt.Println(err)
			server.errCh <- fmt.Errorf("io copy: %w", err)
			return
		}
	}

	go copy(stream, channel)
	go copy(channel, stream)

	<-ctx.Done() // TODO(josebalius): improve this
}

func handleNewChannel(server *Server, channel ssh.Channel) {
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

func (r *rpcHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	handler, found := r.server.services[req.Method]
	if !found {
		r.server.errCh <- fmt.Errorf("RPC Method: '%s' not serviced", req.Method)
		return
	}

	result, err := handler(req)
	if err != nil {
		r.server.errCh <- fmt.Errorf("error handling: '%s': %w", req.Method, err)
		return
	}

	if err := conn.Reply(ctx, req.ID, result); err != nil {
		r.server.errCh <- fmt.Errorf("error replying: %w", err)
	}
}
