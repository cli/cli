package liveshare

import (
	"context"
	"fmt"
)

type Client struct {
	Configuration *Configuration
	SSHSession    *SSHSession
}

func NewClient(configuration *Configuration) *Client {
	return &Client{Configuration: configuration}
}

func (c *Client) Join(ctx context.Context) error {
	session, err := GetSession(ctx, c.Configuration)
	if err != nil {
		return fmt.Errorf("error getting session: %v", err)
	}

	c.SSHSession, err = NewSSH(session).NewSession()
	if err != nil {
		return fmt.Errorf("error connecting to ssh session: %v", err)
	}

	return nil
}
