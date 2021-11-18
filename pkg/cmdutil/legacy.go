package cmdutil

import (
	"fmt"
	"os"

	"github.com/cli/cli/v2/internal/config"
)

// TODO: consider passing via Factory
// TODO: support per-hostname settings
func DetermineEditor(cf func() (config.Config, error)) (string, error) {
	editorCommand := os.Getenv("GH_EDITOR")
	if editorCommand == "" {
		cfg, err := cf()
		if err != nil {
			return "", fmt.Errorf("could not read config: %w", err)
		}
		editorCommand, _ = cfg.Get("", "editor")
	}

	return editorCommand, nil
}
