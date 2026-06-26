package codespaces_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yourorg/yourrepo/internal/codespaces"
)

func TestSSHCommandInjectionSecurity(t *testing.T) {
	payloads := []string{
		// Exact exploit case - command injection payload
		"validuser; rm -rf /",
		// Boundary case - multiple commands with special chars
		"$(cat /etc/passwd)",
		// Valid input - normal username
		"alice",
	}

	for _, payload := range payloads {
		t.Run(payload, func(t *testing.T) {
			// Call the actual production function that creates SSH command
			cmd := codespaces.CreateSSHCommand(payload, "host.example.com", 22)
			
			// Security property: Command must be static, not built from user input
			// We verify by checking that the command path is ssh and arguments don't contain injection
			assert.Equal(t, "ssh", cmd.Path, "Command must be ssh binary")
			
			// Check that user input appears as a single argument, not parsed as shell commands
			hasUserArg := false
			for _, arg := range cmd.Args {
				if strings.Contains(arg, payload) {
					hasUserArg = true
					// Verify the payload appears as a complete argument, not concatenated
					assert.Equal(t, payload, arg, 
						"User input must be passed as complete argument, not concatenated")
				}
			}
			assert.True(t, hasUserArg, "User input must appear in command arguments")
			
			// Ensure no shell execution
			assert.Nil(t, cmd.SysProcAttr, "Should not have shell execution attributes")
			assert.Equal(t, exec.Command("ssh").Path, cmd.Path, 
				"Must use direct exec.Command, not shell execution")
		})
	}
}