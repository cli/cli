package reviewthread

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdReviewThread(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	factory := &cmdutil.Factory{
		IOStreams: ios,
	}

	cmd := NewCmdReviewThread(factory)

	assert.Equal(t, "review-thread <command>", cmd.Use)
	assert.Equal(t, "Manage pull request review threads", cmd.Short)
	assert.True(t, cmd.HasSubCommands())

	// Verify all subcommands are present
	subcommands := cmd.Commands()
	assert.Equal(t, 3, len(subcommands))

	subcommandNames := make(map[string]bool)
	for _, subcmd := range subcommands {
		subcommandNames[subcmd.Name()] = true
	}

	assert.True(t, subcommandNames["resolve"], "resolve subcommand should exist")
	assert.True(t, subcommandNames["unresolve"], "unresolve subcommand should exist")
	assert.True(t, subcommandNames["list"], "list subcommand should exist")
}
