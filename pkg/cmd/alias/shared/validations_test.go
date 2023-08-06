package shared

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestValidAliasNameFunc(t *testing.T) {
	// Create fake command structure for testing.
	issueCmd := &cobra.Command{Use: "issue"}
	prCmd := &cobra.Command{Use: "pr"}
	prCmd.AddCommand(&cobra.Command{Use: "checkout"})

	cmd := &cobra.Command{}
	cmd.AddCommand(prCmd)
	cmd.AddCommand(issueCmd)

	f := ValidAliasNameFunc(cmd)

	assert.False(t, f("pr"))
	assert.False(t, f("pr checkout"))
	assert.False(t, f("issue"))
	assert.False(t, f("repo list"))

	assert.True(t, f("ps"))
	assert.True(t, f("checkout"))
	assert.True(t, f("issue erase"))
	assert.True(t, f("pr erase"))
	assert.True(t, f("pr checkout branch"))
}

func TestValidAliasExpansionFunc(t *testing.T) {
	// Create fake command structure for testing.
	issueCmd := &cobra.Command{Use: "issue"}
	prCmd := &cobra.Command{Use: "pr"}
	prCmd.AddCommand(&cobra.Command{Use: "checkout"})

	cmd := &cobra.Command{}
	cmd.AddCommand(prCmd)
	cmd.AddCommand(issueCmd)

	f := ValidAliasExpansionFunc(cmd)

	assert.False(t, f("ps"))
	assert.False(t, f("checkout"))
	assert.False(t, f("repo list"))

	assert.True(t, f("!git branch --show-current"))
	assert.True(t, f("pr"))
	assert.True(t, f("pr checkout"))
	assert.True(t, f("issue"))
}
