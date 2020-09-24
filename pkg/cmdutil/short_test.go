package cmdutil

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCreateCuratedShortDescription(t *testing.T) {
	exampleCommands := map[string]*cobra.Command{
		"list":   &cobra.Command{Use: "list"},
		"view":   &cobra.Command{Use: "view"},
		"create": &cobra.Command{Use: "create"},
		"edit":   &cobra.Command{Use: "edit"},
		"delete": &cobra.Command{Use: "delete"},
	}

	tests := []struct {
		name        string
		subCommands []string
		curatedList []string
		resource    string
		want        string
	}{
		{
			name:        "Lists only available subcommand",
			subCommands: []string{"list"},
			curatedList: []string{"list"},
			resource:    "Somethings",
			want:        "list Somethings",
		},
		{
			name:        "Lists 2 subcommands",
			subCommands: []string{"list", "view"},
			curatedList: []string{"list", "view"},
			resource:    "Somethings",
			want:        "list and view Somethings",
		},
		{
			name:        "Lists 3 subcommands",
			subCommands: []string{"list", "view", "create"},
			curatedList: []string{"list", "view", "create"},
			resource:    "Somethings",
			want:        "list, view and create Somethings",
		},
		{
			name:        "Describes 1 additional subcommand",
			subCommands: []string{"list", "view", "create", "delete"},
			curatedList: []string{"list", "view", "create"},
			resource:    "Somethings",
			want:        "list, view and create Somethings (+1 more)",
		},
		{
			name:        "Describes 2 additional subcommands",
			subCommands: []string{"list", "view", "create", "edit", "delete"},
			curatedList: []string{"list", "view", "create"},
			resource:    "Somethings",
			want:        "list, view and create Somethings (+2 more)",
		},
		{
			name:        "Filters command that does not exist",
			subCommands: []string{"list", "view"},
			curatedList: []string{"list", "does-not-exist", "view"},
			resource:    "Somethings",
			want:        "list and view Somethings",
		},
		{
			name:        "Does not change Short when no commands are provided",
			subCommands: []string{"list", "view"},
			curatedList: []string{},
			resource:    "Somethings",
			want:        "Default Short description.",
		},
		{
			name:        "Allows case-change in command list",
			subCommands: []string{"list", "view"},
			curatedList: []string{"List", "view"},
			resource:    "Somethings",
			want:        "List and view Somethings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Short: "Default Short description.",
			}

			for _, subCommand := range tt.subCommands {
				cmd.AddCommand(exampleCommands[subCommand])
			}

			ApplyCuratedShort(cmd, tt.resource, tt.curatedList)

			if got := cmd.Short; string(got) != tt.want {
				t.Errorf("ApplyCuratedShort() = %v, want %v", string(got), tt.want)
			}
		})
	}
}
