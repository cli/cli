package liveshare

import (
	"context"
	"fmt"

	"golang.org/x/crypto/ssh"
)

type Client struct {
	liveShare  *LiveShare
	session    *session
	sshSession *sshSession
	rpc        *rpc
}

// NewClient is a function ...
func (l *LiveShare) NewClient() *Client {
	return &Client{liveShare: l}
}

func (c *Client) Join(ctx context.Context) (err error) {
	api := newAPI(c)

	c.session = newSession(api)
	if err := c.session.init(ctx); err != nil {
		return fmt.Errorf("error creating session: %v", err)
	}

	websocket := newWebsocket(c.session)
	if err := websocket.connect(ctx); err != nil {
		return fmt.Errorf("error connecting websocket: %v", err)
	}

	c.sshSession = newSSH(c.session, websocket)
	if err := c.sshSession.connect(ctx); err != nil {
		return fmt.Errorf("error connecting to ssh session: %v", err)
	}

	c.rpc = newRPC(c.sshSession)
	c.rpc.connect(ctx)

	_, err = c.joinWorkspace(ctx)
	if err != nil {
		return fmt.Errorf("error joining liveshare workspace: %v", err)
	}

	return nil
}

func (c *Client) hasJoined() bool {
	return c.sshSession != nil && c.rpc != nil
}

type clientCapabilities struct {
	IsNonInteractive bool `json:"isNonInteractive"`
}

type joinWorkspaceArgs struct {
	ID                      string             `json:"id"`
	ConnectionMode          string             `json:"connectionMode"`
	JoiningUserSessionToken string             `json:"joiningUserSessionToken"`
	ClientCapabilities      clientCapabilities `json:"clientCapabilities"`
}

type joinWorkspaceResult struct {
	SessionNumber int `json:"sessionNumber"`
}

func (c *Client) joinWorkspace(ctx context.Context) (*joinWorkspaceResult, error) {
	args := joinWorkspaceArgs{
		ID:                      c.session.workspaceInfo.ID,
		ConnectionMode:          "local",
		JoiningUserSessionToken: c.session.workspaceAccess.SessionToken,
		ClientCapabilities: clientCapabilities{
			IsNonInteractive: false,
		},
	}

	var result joinWorkspaceResult
	if err := c.rpc.do(ctx, "workspace.joinWorkspace", &args, &result); err != nil {
		return nil, fmt.Errorf("error making workspace.joinWorkspace call: %v", err)
	}

	return &result, nil
}

func (c *Client) openStreamingChannel(ctx context.Context, streamName, condition string) (ssh.Channel, error) {
	args := getStreamArgs{streamName, condition}
	var streamID string
	if err := c.rpc.do(ctx, "streamManager.getStream", args, &streamID); err != nil {
		return nil, fmt.Errorf("error getting stream id: %v", err)
	}

	channel, reqs, err := c.sshSession.conn.OpenChannel("session", nil)
	if err != nil {
		return nil, fmt.Errorf("error opening ssh channel for transport: %v", err)
	}
	go c.processChannelRequests(ctx, reqs)

	requestType := fmt.Sprintf("stream-transport-%s", streamID)
	_, err = channel.SendRequest(requestType, true, nil)
	if err != nil {
		return nil, fmt.Errorf("error sending channel request: %v", err)
	}

	return channel, nil
}

func (c *Client) processChannelRequests(ctx context.Context, reqs <-chan *ssh.Request) {
	for {
		select {
		case req := <-reqs:
			if req != nil {
				// TODO(josebalius): Handle
			}
		case <-ctx.Done():
			break
		}
	}
}
