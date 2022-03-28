package liveshare

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sourcegraph/jsonrpc2"
	"golang.org/x/sync/errgroup"
)

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
	HasTLSHandshakePassed            bool   `json:"hasTLSHandshakePassed"`
	Privacy                          string `json:"privacy"`
}

type PortChangeKind string

const (
	PortChangeKindStart  PortChangeKind = "start"
	PortChangeKindUpdate PortChangeKind = "update"
)

// startSharing tells the Live Share host to start sharing the specified port from the container.
// The sessionName describes the purpose of the remote port or service.
// It returns an identifier that can be used to open an SSH channel to the remote port.
func (s *Session) startSharing(ctx context.Context, sessionName string, port int) (channelID, error) {
	args := []interface{}{port, sessionName, fmt.Sprintf("http://localhost:%d", port)}
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		startNotification, err := s.WaitForPortNotification(ctx, port, PortChangeKindStart)
		if err != nil {
			return fmt.Errorf("error while waiting for port notification: %w", err)

		}
		if !startNotification.Success {
			return fmt.Errorf("error while starting port sharing: %s", startNotification.ErrorDetail)
		}
		return nil // success
	})

	var response Port
	g.Go(func() error {
		return s.rpc.do(ctx, "serverSharing.startSharing", args, &response)
	})

	if err := g.Wait(); err != nil {
		return channelID{}, err
	}

	return channelID{response.StreamName, response.StreamCondition}, nil
}

type PortNotification struct {
	Success bool // Helps us disambiguate between the SharingSucceeded/SharingFailed events
	// The following are properties included in the SharingSucceeded/SharingFailed events sent by the server sharing service in the Codespace
	Port        int            `json:"port"`
	ChangeKind  PortChangeKind `json:"changeKind"`
	ErrorDetail string         `json:"errorDetail"`
	StatusCode  int            `json:"statusCode"`
}

// WaitForPortNotification waits for a port notification to be received. It returns the notification
// or an error if the notification is not received before the context is cancelled or it fails
// to parse the notification.
func (s *Session) WaitForPortNotification(ctx context.Context, port int, notifType PortChangeKind) (*PortNotification, error) {
	// We use 1-buffered channels and non-blocking sends so that
	// no goroutine gets stuck.
	notificationCh := make(chan *PortNotification, 1)
	errCh := make(chan error, 1)

	h := func(success bool) func(*jsonrpc2.Conn, *jsonrpc2.Request) {
		return func(conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
			notification := new(PortNotification)
			if err := json.Unmarshal(*req.Params, &notification); err != nil {
				select {
				case errCh <- fmt.Errorf("error unmarshaling notification: %w", err):
				default:
				}
				return
			}
			notification.Success = success
			if notification.Port == port && notification.ChangeKind == notifType {
				select {
				case notificationCh <- notification:
				default:
				}
			}
		}
	}
	deregisterSuccess := s.registerRequestHandler("serverSharing.sharingSucceeded", h(true))
	deregisterFailure := s.registerRequestHandler("serverSharing.sharingFailed", h(false))
	defer deregisterSuccess()
	defer deregisterFailure()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-errCh:
			return nil, err
		case notification := <-notificationCh:
			return notification, nil
		}
	}
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

// UpdateSharedServerPrivacy controls port permissions and visibility scopes for who can access its URLs
// in the browser.
func (s *Session) UpdateSharedServerPrivacy(ctx context.Context, port int, visibility string) error {
	return s.rpc.do(ctx, "serverSharing.updateSharedServerPrivacy", []interface{}{port, visibility}, nil)
}
