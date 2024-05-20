package cmdutil

import (
	"fmt"
	"os"

	"github.com/cli/cli/v2/internal/gh"
)

// TODO: consider passing via Factory
// TODO: support per-hostname settings
func DetermineEditor(cf func() (gh.Config, error)) (string, error) {
	editorCommand := os.Getenv("GH_EDITOR")
	if editorCommand == "" {
		cfg, err := cf()
		if err != nil {
			return "", fmt.Errorf("could not read config: %w", err)
		}
		editorCommand = cfg.Editor("").Value
	}

	return editorCommand, nil
}
