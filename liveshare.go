package liveshare

import (
	"context"
	"fmt"
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

	return &LiveShare{configuration}, nil
}

func (l *LiveShare) Connect(ctx context.Context) error {
	workspaceClient := NewClient(l.Configuration)
	if err := workspaceClient.Join(ctx); err != nil {
		return fmt.Errorf("error joining with workspace client: %v", err)
	}

	return nil
}
