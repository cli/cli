package liveshare

import (
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

	return &LiveShare{Configuration: configuration}, nil
}
