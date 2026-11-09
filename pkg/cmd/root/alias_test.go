package root

import (
	"errors"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandAlias(t *testing.T) {
	tests := []struct {
		name         string
		expansion    string
		args         []string
		wantExpanded []string
		wantErr      string
	}{
		{
			name:         "no expansion",
			expansion:    "pr status",
			args:         []string{},
			wantExpanded: []string{"pr", "status"},
		},
		{
			name:         "adding arguments after expansion",
			expansion:    "pr checkout",
			args:         []string{"123"},
			wantExpanded: []string{"pr", "checkout", "123"},
		},
		{
			name:      "not enough arguments for expansion",
			expansion: `issue list --author="$1" --label="$2"`,
			args:      []string{},
			wantErr:   `not enough arguments for alias: issue list --author="$1" --label="$2"`,
		},
		{
			name:      "not enough arguments for expansion 2",
			expansion: `issue list --author="$1" --label="$2"`,
			args:      []string{"vilmibm"},
			wantErr:   `not enough arguments for alias: issue list --author="vilmibm" --label="$2"`,
		},
		{
			name:         "satisfy expansion arguments",
			expansion:    `issue list --author="$1" --label="$2"`,
			args:         []string{"vilmibm", "help wanted"},
			wantExpanded: []string{"issue", "list", "--author=vilmibm", "--label=help wanted"},
		},
		{
			name:         "mixed positional and non-positional arguments",
			expansion:    `issue list --author="$1" --label="$2"`,
			args:         []string{"vilmibm", "epic", "-R", "monalisa/testing"},
			wantExpanded: []string{"issue", "list", "--author=vilmibm", "--label=epic", "-R", "monalisa/testing"},
		},
		{
			name:         "dollar in expansion",
			expansion:    `issue list --author="$1" --assignee="$1"`,
			args:         []string{"$coolmoney$"},
			wantExpanded: []string{"issue", "list", "--author=$coolmoney$", "--assignee=$coolmoney$"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExpanded, err := expandAlias(tt.expansion, tt.args)
			if tt.wantErr != "" {
				assert.Nil(t, gotExpanded)
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantExpanded, gotExpanded)
		})
	}
}

func TestExpandShellAlias(t *testing.T) {
	findShFunc := func() (string, error) {
		return "/usr/bin/sh", nil
	}
	tests := []struct {
		name         string
		expansion    string
		args         []string
		findSh       func() (string, error)
		wantExpanded []string
		wantErr      string
	}{
		{
			name:         "simple expansion",
			expansion:    "!git branch --show-current",
			args:         []string{},
			findSh:       findShFunc,
			wantExpanded: []string{"/usr/bin/sh", "-c", "git branch --show-current"},
		},
		{
			name:         "adding arguments after expansion",
			expansion:    "!git branch checkout",
			args:         []string{"123"},
			findSh:       findShFunc,
			wantExpanded: []string{"/usr/bin/sh", "-c", "git branch checkout", "--", "123"},
		},
		{
			name:      "unable to find sh",
			expansion: "!git branch --show-current",
			args:      []string{},
			findSh: func() (string, error) {
				return "", errors.New("unable to locate sh")
			},
			wantErr: "unable to locate sh",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExpanded, err := expandShellAlias(tt.expansion, tt.args, tt.findSh)
			if tt.wantErr != "" {
				assert.Nil(t, gotExpanded)
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantExpanded, gotExpanded)
		})
	}
}

func TestNewCmdAliasSelfExpansion(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	root := &cobra.Command{Use: "gh"}
	issue := &cobra.Command{Use: "issue", Run: func(*cobra.Command, []string) {}}
	issue.AddGroup(&cobra.Group{ID: "alias", Title: "Alias commands"})
	root.AddCommand(issue)
	issue.AddCommand(NewCmdAlias(ios, "mine", "issue mine"))

	root.SetArgs([]string{"issue", "mine"})
	err := root.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "infinite alias expansion")
	require.Contains(t, err.Error(), "gh issue mine")
}

func TestNewCmdAliasIndirectCycle(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	root := &cobra.Command{Use: "gh"}
	root.AddGroup(&cobra.Group{ID: "alias", Title: "Alias commands"})
	root.AddCommand(NewCmdAlias(ios, "foo", "bar"))
	root.AddCommand(NewCmdAlias(ios, "bar", "foo"))

	root.SetArgs([]string{"foo"})
	err := root.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "infinite alias expansion")
}

func TestNewCmdAliasExpandsToExistingCommand(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	root := &cobra.Command{Use: "gh"}
	var ran bool
	issue := &cobra.Command{Use: "issue"}
	issue.AddGroup(&cobra.Group{ID: "alias", Title: "Alias commands"})
	list := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) { ran = true }}
	root.AddCommand(issue)
	issue.AddCommand(list)
	issue.AddCommand(NewCmdAlias(ios, "mine", "issue list"))

	root.SetArgs([]string{"issue", "mine"})
	require.NoError(t, root.Execute())
	require.True(t, ran)
}
