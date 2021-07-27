package liveshare

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

// A Server represents the liveshare host and container server
type Server struct {
	client                      *Client
	port                        int
	streamName, streamCondition string
}

// NewServer creates a new Server with a given Client
func NewServer(client *Client) (*Server, error) {
	if !client.hasJoined() {
		return nil, errors.New("client must join before creating server")
	}

	return &Server{client: client}, nil
}

// Port represents an open port on the container
type Port struct {
	SourcePort                       int    `json:"sourcePort"`
	DestinationPort                  int    `json:"destinationPort"`
	SessionName                      string `json:"sessionName"`
	StreamName                       string `json:"streamName"`
	StreamCondition                  string `json:"streamCondition"`
	BrowseURL                        string `json:"browseUrl"`
	IsPublic                         bool   `json:"isPublic"`
	IsTCPServerConnectionEstablished bool   `json:"isTCPServerConnectionEstablished"`
	HasTSLHandshakePassed            bool   `json:"hasTSLHandshakePassed"`
}

// StartSharing tells the liveshare host to start sharing the port from the container
func (s *Server) StartSharing(ctx context.Context, protocol string, port int) error {
	s.port = port

	var response Port
	if err := s.client.rpc.do(ctx, "serverSharing.startSharing", []interface{}{
		port, protocol, fmt.Sprintf("http://localhost:%s", strconv.Itoa(port)),
	}, &response); err != nil {
		return err
	}

	s.streamName = response.StreamName
	s.streamCondition = response.StreamCondition

	return nil
}

// Ports is a slice of Port pointers
type Ports []*Port

// GetSharedServers returns a list of available/open ports from the container
func (s *Server) GetSharedServers(ctx context.Context) (Ports, error) {
	var response Ports
	if err := s.client.rpc.do(ctx, "serverSharing.getSharedServers", []string{}, &response); err != nil {
		return nil, err
	}

	return response, nil
}

// UpdateSharedVisibility controls port permissions and whether it can be accessed publicly
// via the Browse URL
func (s *Server) UpdateSharedVisibility(ctx context.Context, port int, public bool) error {
	if err := s.client.rpc.do(ctx, "serverSharing.updateSharedServerVisibility", []interface{}{port, public}, nil); err != nil {
		return err
	}

	return nil
}
