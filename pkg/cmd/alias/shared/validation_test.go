package shared

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestValidCommandFunc(t *testing.T) {
	factory := &cmdutil.Factory{
		ExtensionManager: &extensions.ExtensionManagerMock{
			ListFunc: func() []extensions.Extension {
				return []extensions.Extension{}
			},
		},
	}

	// fake command nesting structure needed for validCommand
	issueCmd := &cobra.Command{Use: "issue"}
	prCmd := &cobra.Command{Use: "pr"}
	prCmd.AddCommand(&cobra.Command{Use: "checkout"})

	cmd := &cobra.Command{}
	cmd.AddCommand(prCmd)
	cmd.AddCommand(issueCmd)

	f := ValidCommandFunc(factory, cmd)

	assert.True(t, f("pr"))
	assert.True(t, f("pr checkout"))
	assert.True(t, f("issue"))

	assert.False(t, f("ps"))
	assert.False(t, f("checkout"))
	assert.False(t, f("issu list"))
}
