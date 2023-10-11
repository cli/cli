package connection

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/microsoft/dev-tunnels/go/tunnels"
	tunnelssh "github.com/microsoft/dev-tunnels/go/tunnels/ssh"
	"github.com/microsoft/dev-tunnels/go/tunnels/ssh/messages"
	"golang.org/x/crypto/ssh"
)

func NewMockHttpClient() (*http.Client, error) {
	accessToken := "tunnel access-token"
	relayServer, err := newMockrelayServer(withAccessToken(accessToken))
	if err != nil {
		return nil, fmt.Errorf("NewrelayServer returned an error: %w", err)
	}

	hostURL := strings.Replace(relayServer.URL(), "http://", "ws://", 1)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response []byte
		if r.URL.Path == "/api/v1/tunnels/tunnel-id" {
			tunnel := &tunnels.Tunnel{
				AccessTokens: map[tunnels.TunnelAccessScope]string{
					tunnels.TunnelAccessScopeConnect: accessToken,
				},
				Endpoints: []tunnels.TunnelEndpoint{
					{
						HostID: "host1",
						TunnelRelayTunnelEndpoint: tunnels.TunnelRelayTunnelEndpoint{
							ClientRelayURI: hostURL,
						},
					},
				},
			}

			response, err = json.Marshal(*tunnel)
			if err != nil {
				log.Fatalf("json.Marshal returned an error: %v", err)
			}
		} else if strings.HasPrefix(r.URL.Path, "/api/v1/tunnels/tunnel-id/ports") {
			// Use regex to check if the path ends with a number
			match, err := regexp.MatchString(`\/\d+$`, r.URL.Path)
			if err != nil {
				log.Fatalf("regexp.MatchString returned an error: %v", err)
			}

			// If the path ends with a number, it's a request for a specific port
			if match || r.Method == http.MethodPost {
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusOK)
					return
				}

				tunnelPort := &tunnels.TunnelPort{
					AccessControl: &tunnels.TunnelAccessControl{
						Entries: []tunnels.TunnelAccessControlEntry{},
					},
				}

				// Convert the tunnel to JSON and write it to the response
				response, err = json.Marshal(*tunnelPort)
				if err != nil {
					log.Fatalf("json.Marshal returned an error: %v", err)
				}
			} else {
				// If the path doesn't end with a number and we aren't making a POST request, return an array of ports
				tunnelPorts := []tunnels.TunnelPort{
					{
						AccessControl: &tunnels.TunnelAccessControl{
							Entries: []tunnels.TunnelAccessControlEntry{},
						},
					},
				}

				response, err = json.Marshal(tunnelPorts)
				if err != nil {
					log.Fatalf("json.Marshal returned an error: %v", err)
				}
			}

		} else {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Write the response
		_, _ = w.Write(response)
	}))

	url, err := url.Parse(mockServer.URL)
	if err != nil {
		return nil, fmt.Errorf("url.Parse returned an error: %w", err)
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(url),
		},
	}, nil
}

type relayServer struct {
	httpServer  *httptest.Server
	errc        chan error
	sshConfig   *ssh.ServerConfig
	channels    map[string]channelHandler
	accessToken string

	serverConn *ssh.ServerConn
}

type relayServerOption func(*relayServer)
type channelHandler func(context.Context, ssh.NewChannel) error

func newMockrelayServer(opts ...relayServerOption) (*relayServer, error) {
	server := &relayServer{
		errc: make(chan error),
		sshConfig: &ssh.ServerConfig{
			NoClientAuth: true,
		},
	}

	// Create a private key with the crypto package
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	privateKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		},
	)

	// Parse the private key
	sshPrivateKey, err := ssh.ParsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	server.sshConfig.AddHostKey(ssh.Signer(sshPrivateKey))

	server.httpServer = httptest.NewServer(http.HandlerFunc(makeConnection(server)))

	for _, opt := range opts {
		opt(server)
	}

	return server, nil
}

func withAccessToken(accessToken string) func(*relayServer) {
	return func(server *relayServer) {
		server.accessToken = accessToken
	}
}

func (rs *relayServer) URL() string {
	return rs.httpServer.URL
}

func (rs *relayServer) Err() <-chan error {
	return rs.errc
}

func (rs *relayServer) sendError(err error) {
	select {
	case rs.errc <- err:
	default:
		// channel is blocked with a previous error, so we ignore this one
	}
}

func (rs *relayServer) ForwardPort(ctx context.Context, port uint16) error {
	pfr := messages.NewPortForwardRequest("127.0.0.1", uint32(port))
	b, err := pfr.Marshal()
	if err != nil {
		return fmt.Errorf("error marshaling port forward request: %w", err)
	}

	replied, data, err := rs.serverConn.SendRequest(messages.PortForwardRequestType, true, b)
	if err != nil {
		return fmt.Errorf("error sending port forward request: %w", err)
	}

	if !replied {
		return fmt.Errorf("port forward request not replied")
	}

	if data == nil {
		return fmt.Errorf("no data returned")
	}

	return nil
}

func makeConnection(server *relayServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if server.accessToken != "" {
			if r.Header.Get("Authorization") != server.accessToken {
				server.sendError(fmt.Errorf("invalid access token"))
				return
			}
		}

		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			server.sendError(fmt.Errorf("error upgrading to websocket: %w", err))
			return
		}
		defer func() {
			if err := c.Close(); err != nil {
				server.sendError(fmt.Errorf("error closing websocket: %w", err))
			}
		}()

		socketConn := newSocketConn(c)
		serverConn, chans, reqs, err := ssh.NewServerConn(socketConn, server.sshConfig)
		if err != nil {
			server.sendError(fmt.Errorf("error creating ssh server conn: %w", err))
			return
		}

		go handleRequests(ctx, convertRequests(reqs))

		server.serverConn = serverConn
		if err := handleChannels(ctx, server, chans); err != nil {
			server.sendError(fmt.Errorf("error handling channels: %w", err))
			return
		}
	}
}

func (sr *sshRequest) Type() string {
	return sr.request.Type
}

type sshRequest struct {
	request *ssh.Request
}

// Reply method for sshRequest to satisfy the tunnelssh.SSHRequest interface
func (sr *sshRequest) Reply(success bool, message []byte) error {
	return sr.request.Reply(success, message)
}

// convertRequests function
func convertRequests(reqs <-chan *ssh.Request) <-chan tunnelssh.SSHRequest {
	out := make(chan tunnelssh.SSHRequest)
	go func() {
		for req := range reqs {
			out <- &sshRequest{req}
		}
		close(out)
	}()
	return out
}

func handleChannels(ctx context.Context, server *relayServer, chans <-chan ssh.NewChannel) error {
	errc := make(chan error, 1)
	go func() {
		for ch := range chans {
			if handler, ok := server.channels[ch.ChannelType()]; ok {
				if err := handler(ctx, ch); err != nil {
					errc <- err
					return
				}
			} else {
				// generic accept of the channel to not block
				_, _, err := ch.Accept()
				if err != nil {
					errc <- fmt.Errorf("error accepting channel: %w", err)
					return
				}
			}
		}
	}()
	return awaitError(ctx, errc)
}

func handleRequests(ctx context.Context, reqs <-chan tunnelssh.SSHRequest) {
	for {
		select {
		case <-ctx.Done():
			return
		case req, ok := <-reqs:
			if !ok {
				return
			}

			if req.Type() == "RefreshPorts" {
				_ = req.Reply(true, nil)
				continue
			} else {
				_ = req.Reply(false, nil)
			}
		}
	}
}

func awaitError(ctx context.Context, errc <-chan error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errc:
		return err
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
			return 0, fmt.Errorf("error getting next reader: %w", err)
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
		return 0, fmt.Errorf("error getting next writer: %w", err)
	}

	n, err := w.Write(b)
	if err != nil {
		return 0, fmt.Errorf("error writing: %w", err)
	}

	if err := w.Close(); err != nil {
		return 0, fmt.Errorf("error closing writer: %w", err)
	}

	return n, nil
}

func (s *socketConn) SetDeadline(deadline time.Time) error {
	if err := s.Conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	return s.Conn.SetWriteDeadline(deadline)
}
