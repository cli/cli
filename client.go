package liveshare

import (
	"context"
	"fmt"
)

type Client struct {
	Configuration *Configuration
}

func NewClient(configuration *Configuration) *Client {
	return &Client{configuration}
}

func (c *Client) Join(ctx context.Context) error {
	session, err := GetSession(ctx, c.Configuration)
	if err != nil {
		return fmt.Errorf("error getting session: %v", err)
	}

	sshSession := NewSSHSession(session)
	if err := sshSession.Connect(); err != nil {
		return fmt.Errorf("error authenticating ssh session: %v", err)
	}

	return nil
}
