package context

import (
	"fmt"

	"github.com/cli/cli/internal/config"
)

// NewBlank initializes a blank Context suitable for testing
func NewBlank() *blankContext {
	return &blankContext{}
}

// A Context implementation that queries the filesystem
type blankContext struct {
}

func (c *blankContext) Config() (config.Config, error) {
	cfg, err := config.ParseConfig("config.yml")
	if err != nil {
		panic(fmt.Sprintf("failed to parse config during tests. did you remember to stub? error: %s", err))
	}
	return cfg, nil
}
