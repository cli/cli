package liveshare

import (
	"context"
	"fmt"
)

// A Session represents the session between a connected Live Share client and server.
type Session struct {
	ssh                         *sshSession
	rpc                         *rpcClient
	port                        int
	streamName, streamCondition string
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
	// ^^^
	// TODO(adonovan): fix possible typo in field name, and audit others.
}

// StartSharing tells the liveshare host to start sharing the port from the container
func (s *Session) StartSharing(ctx context.Context, protocol string, port int) error {
	s.port = port

	var response Port
	if err := s.rpc.do(ctx, "serverSharing.startSharing", []interface{}{
		port, protocol, fmt.Sprintf("http://localhost:%d", port),
	}, &response); err != nil {
		return err
	}

	s.streamName = response.StreamName
	s.streamCondition = response.StreamCondition

	return nil
}

// GetSharedServers returns a list of available/open ports from the container
func (s *Session) GetSharedServers(ctx context.Context) ([]*Port, error) {
	var response []*Port
	if err := s.rpc.do(ctx, "serverSharing.getSharedServers", []string{}, &response); err != nil {
		return nil, err
	}

	return response, nil
}

// UpdateSharedVisibility controls port permissions and whether it can be accessed publicly
// via the Browse URL
func (s *Session) UpdateSharedVisibility(ctx context.Context, port int, public bool) error {
	if err := s.rpc.do(ctx, "serverSharing.updateSharedServerVisibility", []interface{}{port, public}, nil); err != nil {
		return err
	}

	return nil
}
