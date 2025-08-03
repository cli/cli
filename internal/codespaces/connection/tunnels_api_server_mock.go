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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/microsoft/dev-tunnels/go/tunnels"
	tunnelssh "github.com/microsoft/dev-tunnels/go/tunnels/ssh"
	"github.com/microsoft/dev-tunnels/go/tunnels/ssh/messages"
	"golang.org/x/crypto/ssh"
)

type mockClientOpts struct {
	ports map[int]tunnels.TunnelPort // Port number to protocol
}

type mockClientOpt func(*mockClientOpts)

// WithSpecificPorts allows you to specify a map of ports to TunnelPorts that will be returned by the mock HTTP client.
// Note that this does not take a copy of the map, so you should not modify the map after passing it to this function.
func WithSpecificPorts(ports map[int]tunnels.TunnelPort) mockClientOpt {
	return func(opts *mockClientOpts) {
		opts.ports = ports
	}
}

func NewMockHttpClient(opts ...mockClientOpt) (*http.Client, error) {
	mockClientOpts := &mockClientOpts{}
	for _, opt := range opts {
		opt(mockClientOpts)
	}

	specifiedPorts := mockClientOpts.ports

	accessToken := "tunnel access-token"
	relayServer, err := newMockrelayServer(withAccessToken(accessToken))
	if err != nil {
		return nil, fmt.Errorf("NewrelayServer returned an error: %w", err)
	}

	hostURL := strings.Replace(relayServer.URL(), "http://", "ws://", 1)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response []byte
		if r.URL.Path == "/tunnels/tunnel-id" {
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

			_, _ = w.Write(response)
			return
		} else if strings.HasPrefix(r.URL.Path, "/tunnels/tunnel-id/ports") {
			// Use regex to capture the port number from the end of the path
			re := regexp.MustCompile(`\/(\d+)$`)
			matches := re.FindStringSubmatch(r.URL.Path)
			targetingSpecificPort := len(matches) > 0

			if targetingSpecificPort {
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusOK)
					return
				}

				if r.Method == http.MethodGet {
					// If no ports were configured, then we assume that every request for a port is valid.
					if specifiedPorts == nil {
						response, err := json.Marshal(tunnels.TunnelPort{
							AccessControl: &tunnels.TunnelAccessControl{
								Entries: []tunnels.TunnelAccessControlEntry{},
							},
						})

						if err != nil {
							log.Fatalf("json.Marshal returned an error: %v", err)
						}

						_, _ = w.Write(response)
						return
					} else {
						// Otherwise we'll fetch the port from our configured ports and include the protocol in the response.
						port, err := strconv.Atoi(matches[1])
						if err != nil {
							log.Fatalf("strconv.Atoi returned an error: %v", err)
						}

						tunnelPort, ok := specifiedPorts[port]
						if !ok {
							w.WriteHeader(http.StatusNotFound)
							return
						}

						response, err := json.Marshal(tunnelPort)

						if err != nil {
							log.Fatalf("json.Marshal returned an error: %v", err)
						}

						_, _ = w.Write(response)
						return
					}
				}

				// Else this is an unexpected request, fall through to 404 at the bottom
			}

			// If it's a PUT request, we assume it's for creating a new port so we'll do some validation
			// and then return a stub.
			if r.Method == http.MethodPut {
				// If a port was already configured with this number, and the protocol has changed, return a 400 Bad Request.
				if specifiedPorts != nil {
					port, err := strconv.Atoi(matches[1])
					if err != nil {
						log.Fatalf("strconv.Atoi returned an error: %v", err)
					}

					var portRequest tunnels.TunnelPort
					if err := json.NewDecoder(r.Body).Decode(&portRequest); err != nil {
						log.Fatalf("json.NewDecoder returned an error: %v", err)
					}

					tunnelPort, ok := specifiedPorts[port]
					if ok {
						if tunnelPort.Protocol != portRequest.Protocol {
							w.WriteHeader(http.StatusBadRequest)
							return
						}
					}

					// Create or update the new port entry.
					specifiedPorts[port] = portRequest
				}

				response, err := json.Marshal(tunnels.TunnelPort{
					AccessControl: &tunnels.TunnelAccessControl{
						Entries: []tunnels.TunnelAccessControlEntry{},
					},
				})

				if err != nil {
					log.Fatalf("json.Marshal returned an error: %v", err)
				}

				_, _ = w.Write(response)
				return
			}

			// Finally, if it's not targeting a specific port or a POST request, we return a list of ports, either
			// totally stubbed, or whatever was configured in the mock client options.
			if specifiedPorts == nil {
				response, err := json.Marshal(tunnels.TunnelPortListResponse{
					Value: []tunnels.TunnelPort{
						{
							AccessControl: &tunnels.TunnelAccessControl{
								Entries: []tunnels.TunnelAccessControlEntry{},
							},
						},
					},
				})
				if err != nil {
					log.Fatalf("json.Marshal returned an error: %v", err)
				}

				_, _ = w.Write(response)
				return
			} else {
				var ports []tunnels.TunnelPort
				for _, tunnelPort := range specifiedPorts {
					ports = append(ports, tunnelPort)
				}
				response, err := json.Marshal(tunnels.TunnelPortListResponse{
					Value: ports,
				})
				if err != nil {
					log.Fatalf("json.Marshal returned an error: %v", err)
				}

				_, _ = w.Write(response)
				return
			}
		} else {
			w.WriteHeader(http.StatusNotFound)
			return
		}
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
