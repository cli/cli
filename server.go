package liveshare

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

type Server struct {
	client                      *Client
	port                        int
	streamName, streamCondition string
}

func (c *Client) NewServer() (*Server, error) {
	if !c.hasJoined() {
		return nil, errors.New("LiveShareClient must join before creating server")
	}

	return &Server{client: c}, nil
}

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

type Ports []*Port

func (s *Server) GetSharedServers(ctx context.Context) (Ports, error) {
	var response Ports
	if err := s.client.rpc.do(ctx, "serverSharing.getSharedServers", []string{}, &response); err != nil {
		return nil, err
	}

	return response, nil
}

func (s *Server) UpdateSharedVisibility(ctx context.Context, port int, public bool) error {
	if err := s.client.rpc.do(ctx, "serverSharing.updateSharedServerVisibility", []interface{}{port, public}, nil); err != nil {
		return err
	}

	return nil
}
