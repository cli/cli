package cmdutil

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// Test_EnableRepoOverride_authCheckIntegration is an integration test for the
// coupling between repo override and the root auth gate, wired through cobra's
// persistent pre-run hooks. EnableRepoOverride replaces a command's
// PersistentPreRunE, so cobra no longer reaches the root auth gate on its own;
// the override hook re-runs the ancestor hooks itself. That re-run must evaluate
// the command the user actually invoked, the same way cobra hands the target
// command to parent hooks:
// https://github.com/spf13/cobra/blob/v1.10.2/command.go#L984-L986
// This pins that a leaf's DisableAuthCheck is honored under a repo-override
// parent.
func Test_EnableRepoOverride_authCheckIntegration(t *testing.T) {
	tests := []struct {
		name            string
		disableAuthLeaf bool
		wantAuthChecked bool
	}{
		{
			name:            "leaf opts out, honored under repo-override parent",
			disableAuthLeaf: true,
			wantAuthChecked: false,
		},
		{
			name:            "leaf does not opt out, auth still checked",
			disableAuthLeaf: false,
			wantAuthChecked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAuthChecked, ranLeaf bool

			// Stand in for the real root auth gate: it judges whatever command
			// it is handed, so it must receive the invoked leaf command.
			root := &cobra.Command{Use: "root"}
			root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
				gotAuthChecked = IsAuthCheckEnabled(cmd)
				return nil
			}

			parent := &cobra.Command{Use: "parent"}
			EnableRepoOverride(parent, &Factory{})
			root.AddCommand(parent)

			leaf := &cobra.Command{
				Use:  "leaf",
				RunE: func(cmd *cobra.Command, args []string) error { ranLeaf = true; return nil },
			}
			if tt.disableAuthLeaf {
				DisableAuthCheck(leaf)
			}
			parent.AddCommand(leaf)

			root.SetArgs([]string{"parent", "leaf"})
			require.NoError(t, root.Execute())

			require.True(t, ranLeaf, "leaf command should have executed")
			require.Equal(t, tt.wantAuthChecked, gotAuthChecked)
		})
	}
}
