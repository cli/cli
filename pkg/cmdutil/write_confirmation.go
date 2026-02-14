package cmdutil

import (
	"fmt"
	"os"
	"strings"
)

// ConfirmWriteCommandsKey is the config key for requiring confirmation on write commands.
// It is exported so that gh config get/set and documentation can reference it.
const ConfirmWriteCommandsKey = "confirm_write_commands"

// RequireWriteConfirmation checks the confirm_write_commands config and environment.
// If confirmation is enabled, it prompts the user to confirm the action (when in an interactive terminal).
// When not in a TTY and confirmation is required, it returns an error explaining how to disable or bypass.
// hostname is the GitHub host (e.g. "github.com"); actionDescription describes what will happen (e.g. "create a pull request").
func RequireWriteConfirmation(f *Factory, hostname, actionDescription string) error {
	confirmRequired := false
	envVal := strings.ToLower(strings.TrimSpace(os.Getenv("GH_CONFIRM_WRITE_COMMANDS")))
	switch envVal {
	case "disabled", "false", "0", "no":
		return nil
	case "enabled", "true", "1", "yes":
		confirmRequired = true
	default:
		// Env unset or unknown: check config
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		entry := cfg.GetOrDefault(hostname, ConfirmWriteCommandsKey)
		if entry.IsNone() {
			return nil
		}
		if strings.ToLower(strings.TrimSpace(entry.Unwrap().Value)) == "enabled" {
			confirmRequired = true
		}
	}
	if !confirmRequired {
		return nil
	}

	// Confirmation is required
	if !f.IOStreams.CanPrompt() {
		return fmt.Errorf(
			"write confirmation is required (confirm_write_commands is enabled). "+
				"To run non-interactively, set GH_CONFIRM_WRITE_COMMANDS=disabled or run: gh config set %s disabled",
			ConfirmWriteCommandsKey,
		)
	}

	ok, err := f.Prompter.Confirm(fmt.Sprintf("This will %s. Continue?", actionDescription), false)
	if err != nil {
		return fmt.Errorf("failed to prompt: %w", err)
	}
	if !ok {
		return CancelError
	}
	return nil
}
