package livesharetest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"
	"golang.org/x/crypto/ssh"
)

type Server struct {
	password string
	services map[string]RpcHandleFunc

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
	b, err := ioutil.ReadFile(filepath.Join("test", "private.key"))
	if err != nil {
		return nil, fmt.Errorf("error reading private.key: %v", err)
	}
	privateKey, err := ssh.ParsePrivateKey(b)
	if err != nil {
		return nil, fmt.Errorf("error parsing key: %v", err)
	}
	server.sshConfig.AddHostKey(privateKey)

	server.errCh = make(chan error)
	server.httptestServer = httptest.NewTLSServer(http.HandlerFunc(newConnection(server)))
	return server, nil
}

type ServerOption func(*Server) error

func WithPassword(password string) ServerOption {
	return func(s *Server) error {
		s.password = password
		return nil
	}
}

func WithService(serviceName string, handler RpcHandleFunc) ServerOption {
	return func(s *Server) error {
		if s.services == nil {
			s.services = make(map[string]RpcHandleFunc)
		}

		s.services[serviceName] = handler
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

func newConnection(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		c, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			server.errCh <- fmt.Errorf("error upgrading connection: %v", err)
			return
		}
		defer c.Close()

		socketConn := newSocketConn(c)
		_, chans, reqs, err := ssh.NewServerConn(socketConn, server.sshConfig)
		if err != nil {
			server.errCh <- fmt.Errorf("error creating new ssh conn: %v", err)
			return
		}
		go ssh.DiscardRequests(reqs)

		for newChannel := range chans {
			ch, reqs, err := newChannel.Accept()
			if err != nil {
				server.errCh <- fmt.Errorf("error accepting new channel: %v", err)
				return
			}
			go ssh.DiscardRequests(reqs)
			go handleNewChannel(server, ch)
		}
	}
}

func handleNewChannel(server *Server, channel ssh.Channel) {
	stream := jsonrpc2.NewBufferedStream(channel, jsonrpc2.VSCodeObjectCodec{})
	jsonrpc2.NewConn(context.Background(), stream, newRpcHandler(server))
}

type RpcHandleFunc func(req *jsonrpc2.Request) (interface{}, error)

type rpcHandler struct {
	server *Server
}

func newRpcHandler(server *Server) *rpcHandler {
	return &rpcHandler{server}
}

func (r *rpcHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	handler, found := r.server.services[req.Method]
	if !found {
		r.server.errCh <- fmt.Errorf("RPC Method: '%v' not serviced", req.Method)
		return
	}

	result, err := handler(req)
	if err != nil {
		r.server.errCh <- fmt.Errorf("error handling: '%v': %v", req.Method, err)
		return
	}

	if err := conn.Reply(ctx, req.ID, result); err != nil {
		r.server.errCh <- fmt.Errorf("error replying: %v", err)
	}
}

type socketConn struct {
	*websocket.Conn

	reader     io.Reader
	writeMutex sync.Mutex
	readMutex  sync.Mutex
}

func newSocketConn(conn *websocket.Conn) *socketConn {
	return &socketConn{Conn: conn}
}

func (s *socketConn) Read(b []byte) (int, error) {
	s.readMutex.Lock()
	defer s.readMutex.Unlock()

	if s.reader == nil {
		msgType, r, err := s.Conn.NextReader()
		if err != nil {
			return 0, fmt.Errorf("error getting next reader: %v", err)
		}
		if msgType != websocket.BinaryMessage {
			return 0, fmt.Errorf("invalid message type")
		}
		s.reader = r
	}

	bytesRead, err := s.reader.Read(b)
	if err != nil {
		s.reader = nil

		if err == io.EOF {
			err = nil
		}
	}

	return bytesRead, err
}

func (s *socketConn) Write(b []byte) (int, error) {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	w, err := s.Conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return 0, fmt.Errorf("error getting next writer: %v", err)
	}

	n, err := w.Write(b)
	if err != nil {
		return 0, fmt.Errorf("error writing: %v", err)
	}

	if err := w.Close(); err != nil {
		return 0, fmt.Errorf("error closing writer: %v", err)
	}

	return n, nil
}

func (s *socketConn) SetDeadline(deadline time.Time) error {
	if err := s.Conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	return s.Conn.SetWriteDeadline(deadline)
}
