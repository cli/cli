package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

func Test_printError(t *testing.T) {
	cmd := &cobra.Command{}

	type args struct {
		err   error
		cmd   *cobra.Command
		debug bool
	}
	tests := []struct {
		name    string
		args    args
		wantOut string
	}{
		{
			name: "generic error",
			args: args{
				err:   errors.New("the app exploded"),
				cmd:   nil,
				debug: false,
			},
			wantOut: "the app exploded\n",
		},
		{
			name: "DNS error",
			args: args{
				err: fmt.Errorf("DNS oopsie: %w", &net.DNSError{
					Name: "api.github.com",
				}),
				cmd:   nil,
				debug: false,
			},
			wantOut: `error connecting to api.github.com
check your internet connection or https://githubstatus.com
`,
		},
		{
			name: "Cobra flag error",
			args: args{
				err:   cmdutil.FlagErrorf("unknown flag --foo"),
				cmd:   cmd,
				debug: false,
			},
			wantOut: "unknown flag --foo\n\nUsage:\n\n",
		},
		{
			name: "unknown Cobra command error",
			args: args{
				err:   errors.New("unknown command foo"),
				cmd:   cmd,
				debug: false,
			},
			wantOut: "unknown command foo\n\nUsage:\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			printError(out, tt.args.err, tt.args.cmd, tt.args.debug)
			if gotOut := out.String(); gotOut != tt.wantOut {
				t.Errorf("printError() = %q, want %q", gotOut, tt.wantOut)
			}
		})
	}
}

func Test_printNoAuthHelp(t *testing.T) {
	orig_CI := os.Getenv("CI")
	orig_GITHUB_ACTIONS := os.Getenv("GITHUB_ACTIONS")
	t.Cleanup(func() {
		os.Setenv("CI", orig_CI)
		os.Setenv("GITHUB_ACTIONS", orig_GITHUB_ACTIONS)
	})
	tests := []struct {
		name           string
		CI             string
		GITHUB_ACTIONS string
		wantOut        string
	}{
		{
			name:           "Interactive",
			CI:             "",
			GITHUB_ACTIONS: "",
			wantOut: `Welcome to GitHub CLI!

To authenticate, please run ` + "`gh auth login`.\n",
		},
		{
			name:           "Non-Interactive, not a GitHub Action",
			CI:             "true",
			GITHUB_ACTIONS: "",
			wantOut: "`GH_TOKEN` must be set to use `gh` in this CI environment." + `

Please consult the relevant documentation for setting these environment variables.
`,
		},
		{
			name:           "GitHub Action",
			CI:             "true",
			GITHUB_ACTIONS: "true",
			wantOut: "`GH_TOKEN` must be set to use `gh` in a GitHub Action." + `

Example:

env:
  GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			cs := &iostreams.ColorScheme{}
			os.Setenv("CI", tt.CI)
			os.Setenv("GITHUB_ACTIONS", tt.GITHUB_ACTIONS)
			printNoAuthHelp(out, cs)
			if gotOut := out.String(); gotOut != tt.wantOut {
				t.Errorf("printNoAuthHelp() = %q, want %q", gotOut, tt.wantOut)
			}
		})
	}
}
