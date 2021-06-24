package liveshare

import (
	"errors"
	"strings"
)

type Option func(configuration *Configuration) error

func WithWorkspaceID(id string) Option {
	return func(configuration *Configuration) error {
		configuration.WorkspaceID = id
		return nil
	}
}

func WithLiveShareEndpoint(liveShareEndpoint string) Option {
	return func(configuration *Configuration) error {
		configuration.LiveShareEndpoint = liveShareEndpoint
		return nil
	}
}

func WithToken(token string) Option {
	return func(configuration *Configuration) error {
		configuration.Token = token
		return nil
	}
}

type Configuration struct {
	WorkspaceID, LiveShareEndpoint, Token string
}

func NewConfiguration() *Configuration {
	return &Configuration{
		LiveShareEndpoint: "https://prod.liveshare.vsengsaas.visualstudio.com",
	}
}

func (c *Configuration) Validate() error {
	errs := []string{}
	if c.WorkspaceID == "" {
		errs = append(errs, "WorkspaceID is required")
	}

	if c.Token == "" {
		errs = append(errs, "Token is required")
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, ", "))
	}

	return nil
}
