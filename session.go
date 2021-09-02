package liveshare

import (
	"context"
	"fmt"
)

// A Session represents the session between a connected Live Share client and server.
type Session struct {
	ssh *sshSession
	rpc *rpcClient
}

// Port describes a port exposed by the container.
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

// startSharing tells the Live Share host to start sharing the specified port from the container.
// The sessionName describes the purpose of the remote port or service.
// It returns an identifier that can be used to open an SSH channel to the remote port.
func (s *Session) startSharing(ctx context.Context, sessionName string, port int) (channelID, error) {
	args := []interface{}{port, sessionName, fmt.Sprintf("http://localhost:%d", port)}
	var response Port
	if err := s.rpc.do(ctx, "serverSharing.startSharing", args, &response); err != nil {
		return channelID{}, err
	}

	return channelID{response.StreamName, response.StreamCondition}, nil
}

// GetSharedServers returns a description of each container port
// shared by a prior call to StartSharing by some client.
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
