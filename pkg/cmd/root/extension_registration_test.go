package root

import (
	"testing"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdRoot_ExtensionRegistration(t *testing.T) {
	tests := []struct {
		name           string
		extensions     []string
		wantRegistered []string
		wantSkipped    []string
	}{
		{
			name:           "extension conflicts with core command 'copilot'",
			extensions:     []string{"copilot"},
			wantSkipped:    []string{"copilot"},
			wantRegistered: []string{},
		},
		{
			name:           "extension does not conflict with any core command",
			extensions:     []string{"my-custom-extension"},
			wantSkipped:    []string{},
			wantRegistered: []string{"my-custom-extension"},
		},
		{
			name:           "extension that conflicts with a core command's alias",
			extensions:     []string{"agent"},
			wantSkipped:    []string{"agent"},
			wantRegistered: []string{},
		},
		{
			name:           "multiple extensions with some conflicts",
			extensions:     []string{"pr", "custom-ext", "issue", "another-ext"},
			wantSkipped:    []string{"pr", "issue"},
			wantRegistered: []string{"custom-ext", "another-ext"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()

			var extMocks []extensions.Extension
			for _, extName := range tt.extensions {
				extMocks = append(extMocks, &extensions.ExtensionMock{
					NameFunc: func() string {
						return extName
					},
				})
			}

			em := &extensions.ExtensionManagerMock{
				ListFunc: func() []extensions.Extension {
					return extMocks
				},
			}

			f := &cmdutil.Factory{
				IOStreams: ios,
				Config: func() (gh.Config, error) {
					return config.NewBlankConfig(), nil
				},
				Browser:          &browser.Stub{},
				ExtensionManager: em,
			}

			cmd, err := NewCmdRoot(f, "", "")
			require.NoError(t, err)

			// Verify skipped extensions (should find core command registered, not extension)
			for _, extName := range tt.wantSkipped {
				foundCmd, _, findErr := cmd.Find([]string{extName})
				assert.NoError(t, findErr, "command %q should be found", extName)
				assert.NotNil(t, foundCmd, "command %q should exist", extName)
				assert.NotEqual(t, "extension", foundCmd.GroupID, "command %q should be core command, not extension", extName)
			}

			// Verify registered extensions (should find extension command registered)
			for _, extName := range tt.wantRegistered {
				foundCmd, _, findErr := cmd.Find([]string{extName})
				assert.NoError(t, findErr, "extension %q should be found", extName)
				assert.NotNil(t, foundCmd, "extension %q should exist", extName)
				assert.Equal(t, "extension", foundCmd.GroupID, "command %q should be extension command", extName)
			}
		})
	}
}
