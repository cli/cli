package liveshare

import (
	"errors"
	"fmt"
	"strings"
)

type LiveShare struct {
	Configuration *Configuration
}

func New(opts ...Option) (*LiveShare, error) {
	configuration := NewConfiguration()

	for _, o := range opts {
		if err := o(configuration); err != nil {
			return nil, fmt.Errorf("error configuring liveshare: %v", err)
		}
	}

	if err := configuration.Validate(); err != nil {
		return nil, fmt.Errorf("error validating configuration: %v", err)
	}

	return &LiveShare{Configuration: configuration}, nil
}

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
