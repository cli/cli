package command

import (
	"testing"

	"github.com/spf13/pflag"
)

func TestPrepareCommandArguments(t *testing.T) {
	// A few sample commands, just to ensure that flags and arguments are being set properly
	tests := []struct {
		testName string
		cmd      string
		cmdName  string
		args     []string
		flags    map[string]string // TODO: Convert this into a map[string]ValueWithType
	}{
		{
			testName: "Root Command",
			cmd:      "",
			cmdName:  "gh",
			args:     []string{},
			flags:    map[string]string{},
		},
		{
			testName: "Help Flag",
			cmd:      "--help",
			cmdName:  "gh",
			args:     []string{},
			flags:    map[string]string{"help": "true"},
		},
		{
			testName: "SubCommand",
			cmd:      "issue",
			cmdName:  "issue",
			args:     []string{},
			flags:    map[string]string{},
		},
		{
			testName: "SubCommand with help flag",
			cmd:      "issue view --help",
			cmdName:  "view",
			args:     []string{},
			flags:    map[string]string{"help": "true"},
		},
		{
			testName: "SubCommand with argument",
			cmd:      "issue view 23",
			cmdName:  "view",
			args:     []string{"23"},
			flags:    map[string]string{},
		},
		{
			testName: "SubCommand with argument and flag",
			cmd:      "issue view 23 -w",
			cmdName:  "view",
			args:     []string{"23"},
			flags:    map[string]string{"web": "true"},
		},
		{
			testName: "SubCommand with argument, flag and global flag",
			cmd:      "issue view 23 -w -R OWNER/REPO",
			cmdName:  "view",
			args:     []string{"23"},
			flags:    map[string]string{"web": "true", "repo": "OWNER/REPO"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			cmd, args, _, cleanUpFunc, err := PrepareCommandArguments(tt.cmd)
			if err != nil {
				t.Errorf("error testing PrepareCommandArguments - Test: %s: %v", tt.testName, err)
			}
			defer cleanUpFunc()
			eq(t, cmd.Name(), tt.cmdName)
			eq(t, args, tt.args)
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				if val, ok := tt.flags[f.Name]; ok {
					eq(t, f.Value.String(), val)
				} else {
					eq(t, f.Value.String(), f.DefValue)
				}
			})
		})
	}
}
