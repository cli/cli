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

func TestNewCmdAlias_cyclicRecursion(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	rootCmd := &cobra.Command{Use: "gh"}
	rootCmd.AddGroup(&cobra.Group{ID: "alias", Title: "Alias commands"})
	aliasCmd := NewCmdAlias(ios, "mine", "mine")
	rootCmd.AddCommand(aliasCmd)
	rootCmd.SetArgs([]string{"mine"})
	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "infinite alias expansion: gh mine -> gh mine")
}

func TestNewCmdAlias_mutualRecursion(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	rootCmd := &cobra.Command{Use: "gh"}
	rootCmd.AddGroup(&cobra.Group{ID: "alias", Title: "Alias commands"})
	fooCmd := NewCmdAlias(ios, "foo", "bar")
	barCmd := NewCmdAlias(ios, "bar", "foo")
	rootCmd.AddCommand(fooCmd)
	rootCmd.AddCommand(barCmd)
	rootCmd.SetArgs([]string{"foo"})
	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "infinite alias expansion: gh foo -> gh bar -> gh foo")
}

func TestNewCmdAlias_nonCyclic(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	rootCmd := &cobra.Command{Use: "gh"}
	rootCmd.AddGroup(&cobra.Group{ID: "alias", Title: "Alias commands"})
	var executed bool
	targetCmd := &cobra.Command{
		Use: "target",
		Run: func(cmd *cobra.Command, args []string) {
			executed = true
		},
	}
	aliasCmd := NewCmdAlias(ios, "alias-target", "target")
	rootCmd.AddCommand(targetCmd)
	rootCmd.AddCommand(aliasCmd)
	rootCmd.SetArgs([]string{"alias-target"})
	err := rootCmd.Execute()
	require.NoError(t, err)
	assert.True(t, executed)
}
