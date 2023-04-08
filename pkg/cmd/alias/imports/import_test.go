package imports

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAliasImport(t *testing.T) {
	tests := []struct {
		name             string
		isTTY            bool
		flags            []string
		stdin            string
		fileContents     string
		initialConfig    string
		expectedConfig   string
		expectedOutLines []string
		expectedErrLines []string
	}{
		{
			name:  "with no existing aliases",
			isTTY: true,
			flags: []string{},
			fileContents: heredoc.Doc(`
                co: pr checkout
                igrep: '!gh issue list --label="$1" | grep "$2"'
            `),
			expectedConfig: heredoc.Doc(`
                aliases:
                    co: pr checkout
                    igrep: '!gh issue list --label="$1" | grep "$2"'
            `),
			expectedErrLines: []string{"Importing aliases from file", "Added alias (co|igrep)"},
			expectedOutLines: []string{},
		},
		{
			name:  "with existing aliases",
			isTTY: true,
			flags: []string{},
			fileContents: heredoc.Doc(`
                users: |-
                    api graphql -F name="$1" -f query='
                        query ($name: String!) {
                            user(login: $name) {
                                name
                            }
                        }'
                co: pr checkout
            `),
			initialConfig: heredoc.Doc(`
                aliases:
                    igrep: '!gh issue list --label="$1" | grep "$2"'
                editor: vim
            `),
			expectedConfig: heredoc.Doc(`
                aliases:
                    igrep: '!gh issue list --label="$1" | grep "$2"'
                    co: pr checkout
                    users: |-
                        api graphql -F name="$1" -f query='
                            query ($name: String!) {
                                user(login: $name) {
                                    name
                                }
                            }'
                editor: vim
            `),
			expectedErrLines: []string{"Importing aliases from file", "Added alias (co|users)"},
			expectedOutLines: []string{},
		},
		{
			name:  "from stdin",
			isTTY: true,
			flags: []string{},
			stdin: heredoc.Doc(`
                co: pr checkout
                features: |-
                    issue list
                    --label=enhancement
                igrep: '!gh issue list --label="$1" | grep "$2"'
            `),
			expectedConfig: heredoc.Doc(`
                aliases:
                    co: pr checkout
                    features: |-
                        issue list
                        --label=enhancement
                    igrep: '!gh issue list --label="$1" | grep "$2"'
            `),
			expectedErrLines: []string{"Importing aliases from standard input", "Added alias (co|igrep)"},
			expectedOutLines: []string{},
		},
		{
			name:  "already taken aliases",
			isTTY: true,
			flags: []string{},
			fileContents: heredoc.Doc(`
                co: pr checkout -R cool/repo
                igrep: '!gh issue list --label="$1" | grep "$2"'
            `),
			initialConfig: heredoc.Doc(`
                aliases:
                    co: pr checkout
                editor: vim
            `),
			expectedConfig: heredoc.Doc(`
                aliases:
                    co: pr checkout
                    igrep: '!gh issue list --label="$1" | grep "$2"'
                editor: vim
            `),
			expectedErrLines: []string{
				"Importing aliases from file",
				"Could not import alias co: already taken",
				"Added alias igrep",
			},
			expectedOutLines: []string{},
		},
		{
			name:  "override aliases",
			isTTY: true,
			flags: []string{"--clobber"},
			fileContents: heredoc.Doc(`
                co: pr checkout -R cool/repo
                igrep: '!gh issue list --label="$1" | grep "$2"'
            `),
			initialConfig: heredoc.Doc(`
                aliases:
                    co: pr checkout
                editor: vim
            `),
			expectedConfig: heredoc.Doc(`
                aliases:
                    co: pr checkout -R cool/repo
                    igrep: '!gh issue list --label="$1" | grep "$2"'
                editor: vim
            `),
			expectedErrLines: []string{
				"Importing aliases from file",
				"Changed alias co",
				"Added alias igrep",
			},
			expectedOutLines: []string{},
		},
		{
			name:  "alias is a gh command",
			isTTY: true,
			flags: []string{},
			fileContents: heredoc.Doc(`
                pr: pr checkout
                issue: issue list
                api: api graphql
            `),
			expectedErrLines: []string{"Could not import alias (pr|checkout|list): already a gh command"},
			expectedOutLines: []string{},
		},
		{
			name:  "invalid expansion",
			isTTY: true,
			flags: []string{},
			fileContents: heredoc.Doc(`
                alias1:
                alias2: ps checkout
            `),
			expectedErrLines: []string{"Could not import alias alias[0-9]: expansion does not correspond to a gh command"},
			expectedOutLines: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := "-"
			if tt.stdin == "" {
				tmpFile := filepath.Join(t.TempDir(), "aliases")
				err := os.WriteFile(tmpFile, []byte(tt.fileContents), 0600)
				if err != nil {
					t.Fatalf("could not write test file %v", err)
				}
				cli = tmpFile
			}
			cli = strings.Join(append(tt.flags, cli), " ")

			readConfigs := config.StubWriteConfig(t)

			cfg := config.NewFromString(tt.initialConfig)

			output, err := runCommand(cfg, tt.isTTY, cli, tt.stdin)
			require.NoError(t, err)

			mainBuf := bytes.Buffer{}
			readConfigs(&mainBuf, io.Discard)

			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, output.Stderr(), tt.expectedErrLines...)
			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, output.String(), tt.expectedOutLines...)

			assert.Equal(t, tt.expectedConfig, mainBuf.String())
		})
	}
}

func TestAliasImport_no_filename(t *testing.T) {
	cfg := config.NewFromString("")
	_, err := runCommand(cfg, true, "", "")
	require.EqualError(t, err, "no filename passed and nothing on STDIN")
}

func TestAliasImport_multiple_filenames(t *testing.T) {
	cfg := config.NewFromString("")
	_, err := runCommand(cfg, true, "aliases1 aliases2", "")
	require.EqualError(t, err, "too many arguments")
}

func runCommand(cfg config.Config, isTTY bool, cli, in string) (*test.CmdOut, error) {
	ios, stdin, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)
	stdin.WriteString(in)

	factory := &cmdutil.Factory{
		IOStreams: ios,
		Config: func() (config.Config, error) {
			return cfg, nil
		},
		ExtensionManager: &extensions.ExtensionManagerMock{
			ListFunc: func() []extensions.Extension {
				return []extensions.Extension{}
			},
		},
	}

	cmd := NewCmdImport(factory, nil)

	// fake command nesting structure needed for validCommand
	rootCmd := &cobra.Command{}
	rootCmd.AddCommand(cmd)
	prCmd := &cobra.Command{Use: "pr"}
	prCmd.AddCommand(&cobra.Command{Use: "checkout"})
	prCmd.AddCommand(&cobra.Command{Use: "status"})
	rootCmd.AddCommand(prCmd)
	issueCmd := &cobra.Command{Use: "issue"}
	issueCmd.AddCommand(&cobra.Command{Use: "list"})
	rootCmd.AddCommand(issueCmd)
	apiCmd := &cobra.Command{Use: "api"}
	apiCmd.AddCommand(&cobra.Command{Use: "graphql"})
	rootCmd.AddCommand(apiCmd)

	argv, err := shlex.Split("import " + cli)
	if err != nil {
		return nil, err
	}
	rootCmd.SetArgs(argv)
	rootCmd.SetIn(stdin)
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)

	err = rootCmd.Execute()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}
