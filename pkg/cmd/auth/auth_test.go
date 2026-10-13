package auth

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdAuth_telemetryEnablement(t *testing.T) {
	cmd := NewCmdAuth(&cmdutil.Factory{}, nil)

	// login and refresh opt back into telemetry; every other auth subcommand stays excluded.
	enabled := map[string]bool{
		"login":   true,
		"refresh": true,
	}

	for _, sub := range cmd.Commands() {
		name := sub.Name()
		disabled := sub.Annotations["telemetry"] == "disabled"
		if enabled[name] {
			assert.False(t, disabled, "expected %q to have telemetry enabled", name)
		} else {
			assert.True(t, disabled, "expected %q to have telemetry disabled", name)
		}
	}
}
