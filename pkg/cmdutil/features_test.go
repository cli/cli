package cmdutil

import (
	"os"
	"testing"

	"github.com/cli/cli/v2/internal/gh"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/stretchr/testify/assert"
)

func TestAdvancedIssueSearchEnabled(t *testing.T) {
	tests := []struct {
		name        string
		envVar      string
		configValue string
		configErr   error
		wantEnabled bool
	}{
		{
			name:        "environment variable set to enabled",
			envVar:      "enabled",
			wantEnabled: true,
		},
		{
			name:        "environment variable set to something truthy",
			envVar:      "yesplease",
			wantEnabled: true,
		},
		{
			name:        "environment variable set to disabled",
			envVar:      "disabled",
			wantEnabled: false,
		},
		{
			name:        "environment variable set to false",
			envVar:      "false",
			wantEnabled: false,
		},
		{
			name:        "environment variable set to ''",
			envVar:      "",
			wantEnabled: false,
		},
		{
			name:        "config value enabled",
			envVar:      "[unset]",
			configValue: "enabled",
			wantEnabled: true,
		},
		{
			name:        "config value disabled",
			envVar:      "[unset]",
			configValue: "disabled",
			wantEnabled: false,
		},
		// {
		//     name:        "config value disabled",
		//     envVar:      "",
		//     configValue: "disabled",
		//     wantEnabled: false,
		// },
		// {
		//     name:        "config value other",
		//     envVar:      "",
		//     configValue: "other",
		//     wantEnabled: false,
		// },
		// {
		//     name:        "config error",
		//     envVar:      "",
		//     configErr:   errors.New("config error"),
		//     wantEnabled: false,
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable for test
			oldVal := os.Getenv("GH_ADVANCED_ISSUE_SEARCH")
			if tt.envVar != "[unset]" {
				t.Setenv("GH_ADVANCED_ISSUE_SEARCH", tt.envVar)
			} else {
				os.Unsetenv("GH_ADVANCED_ISSUE_SEARCH")
			}
			t.Cleanup(func() {
				if oldVal != "" {
					os.Setenv("GH_ADVANCED_ISSUE_SEARCH", oldVal)
				} else {
					os.Unsetenv("GH_ADVANCED_ISSUE_SEARCH")
				}
			})

			mockConfig := &ghmock.ConfigMock{}
			mockConfig.AdvancedIssueSearchFunc = func(host string) gh.ConfigEntry {
				return gh.ConfigEntry{Value: tt.configValue}
			}

			factory := &Factory{}
			factory.Config = func() (gh.Config, error) {
				if tt.configErr != nil {
					return nil, tt.configErr
				}
				return mockConfig, nil
			}

			result := AdvancedIssueSearchEnabled(factory)

			assert.Equal(t, tt.wantEnabled, result)
		})
	}
}
