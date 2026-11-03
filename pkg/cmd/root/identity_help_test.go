package root

import (
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthHelpDoesNotAdvertiseUnsupportedAccountOverride(t *testing.T) {
	for _, command := range [][]string{{"auth", "use"}, {"auth", "token"}, {"auth", "identity", "delete"}} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			// Given an auth-management command that rejects the global override
			isolateAccountCommand(t)
			cfg, _ := config.NewIsolatedTestConfig(t, "")
			cmd, out, errOut := accountCommand(t, cfg, &httpmock.Registry{})
			cmd.SetArgs(append(command, "--help"))

			// When the user asks for help
			require.NoError(t, cmd.Execute(), "%s", errOut.String())

			// Then inherited flags do not advertise the unsupported option
			_, flags, _ := strings.Cut(out.String(), "INHERITED FLAGS")
			flags, _, _ = strings.Cut(flags, "\n\n")
			assert.NotContains(t, flags, "--account")
		})
	}
}
