package set

import (
	"bytes"
	"io/ioutil"
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

func runCommand(cfg config.Config, isTTY bool, cli string, in string) (*test.CmdOut, error) {
	io, stdin, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(isTTY)
	io.SetStdinTTY(isTTY)
	io.SetStderrTTY(isTTY)
	stdin.WriteString(in)

	factory := &cmdutil.Factory{
		IOStreams: io,
		Config: func() (config.Config, error) {
			return cfg, nil
		},
		ExtensionManager: &extensions.ExtensionManagerMock{
			ListFunc: func(bool) []extensions.Extension {
				return []extensions.Extension{}
			},
		},
	}

	cmd := NewCmdSet(factory, nil)

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

	argv, err := shlex.Split("set " + cli)
	if err != nil {
		return nil, err
	}
	rootCmd.SetArgs(argv)

	rootCmd.SetIn(stdin)
	rootCmd.SetOut(ioutil.Discard)
	rootCmd.SetErr(ioutil.Discard)

	_, err = rootCmd.ExecuteC()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

func TestAliasSet_gh_command(t *testing.T) {
	defer config.StubWriteConfig(ioutil.Discard, ioutil.Discard)()

	cfg := config.NewFromString(``)

	_, err := runCommand(cfg, true, "pr 'pr status'", "")
	assert.EqualError(t, err, `could not create alias: "pr" is already a gh command`)
}

func TestAliasSet_empty_aliases(t *testing.T) {
	mainBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, ioutil.Discard)()

	cfg := config.NewFromString(heredoc.Doc(`
		aliases:
		editor: vim
	`))

	output, err := runCommand(cfg, true, "co 'pr checkout'", "")

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), "Added alias")
	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.String(), "")

	expected := `aliases:
    co: pr checkout
editor: vim
`
	assert.Equal(t, expected, mainBuf.String())
}

func TestAliasSet_existing_alias(t *testing.T) {
	mainBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, ioutil.Discard)()

	cfg := config.NewFromString(heredoc.Doc(`
		aliases:
		  co: pr checkout
	`))

	output, err := runCommand(cfg, true, "co 'pr checkout -Rcool/repo'", "")
	require.NoError(t, err)

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), "Changed alias.*co.*from.*pr checkout.*to.*pr checkout -Rcool/repo")
}

func TestAliasSet_space_args(t *testing.T) {
	mainBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, ioutil.Discard)()

	cfg := config.NewFromString(``)

	output, err := runCommand(cfg, true, `il 'issue list -l "cool story"'`, "")
	require.NoError(t, err)

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), `Adding alias for.*il.*issue list -l "cool story"`)

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, mainBuf.String(), `il: issue list -l "cool story"`)
}

func TestAliasSet_arg_processing(t *testing.T) {
	cases := []struct {
		Cmd                string
		ExpectedOutputLine string
		ExpectedConfigLine string
	}{
		{`il "issue list"`, "- Adding alias for.*il.*issue list", "il: issue list"},

		{`iz 'issue list'`, "- Adding alias for.*iz.*issue list", "iz: issue list"},

		{`ii 'issue list --author="$1" --label="$2"'`,
			`- Adding alias for.*ii.*issue list --author="\$1" --label="\$2"`,
			`ii: issue list --author="\$1" --label="\$2"`},

		{`ix "issue list --author='\$1' --label='\$2'"`,
			`- Adding alias for.*ix.*issue list --author='\$1' --label='\$2'`,
			`ix: issue list --author='\$1' --label='\$2'`},
	}

	for _, c := range cases {
		t.Run(c.Cmd, func(t *testing.T) {
			mainBuf := bytes.Buffer{}
			defer config.StubWriteConfig(&mainBuf, ioutil.Discard)()

			cfg := config.NewFromString(``)

			output, err := runCommand(cfg, true, c.Cmd, "")
			if err != nil {
				t.Fatalf("got unexpected error running %s: %s", c.Cmd, err)
			}

			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, output.Stderr(), c.ExpectedOutputLine)
			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, mainBuf.String(), c.ExpectedConfigLine)
		})
	}
}

func TestAliasSet_init_alias_cfg(t *testing.T) {
	mainBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, ioutil.Discard)()

	cfg := config.NewFromString(heredoc.Doc(`
		editor: vim
	`))

	output, err := runCommand(cfg, true, "diff 'pr diff'", "")
	require.NoError(t, err)

	expected := `editor: vim
aliases:
    diff: pr diff
`

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), "Adding alias for.*diff.*pr diff", "Added alias.")
	assert.Equal(t, expected, mainBuf.String())
}

func TestAliasSet_existing_aliases(t *testing.T) {
	mainBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, ioutil.Discard)()

	cfg := config.NewFromString(heredoc.Doc(`
		aliases:
		  foo: bar
	`))

	output, err := runCommand(cfg, true, "view 'pr view'", "")
	require.NoError(t, err)

	expected := `aliases:
    foo: bar
    view: pr view
`

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), "Adding alias for.*view.*pr view", "Added alias.")
	assert.Equal(t, expected, mainBuf.String())

}

func TestAliasSet_invalid_command(t *testing.T) {
	defer config.StubWriteConfig(ioutil.Discard, ioutil.Discard)()

	cfg := config.NewFromString(``)

	_, err := runCommand(cfg, true, "co 'pe checkout'", "")
	assert.EqualError(t, err, "could not create alias: pe checkout does not correspond to a gh command")
}

func TestShellAlias_flag(t *testing.T) {
	mainBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, ioutil.Discard)()

	cfg := config.NewFromString(``)

	output, err := runCommand(cfg, true, "--shell igrep 'gh issue list | grep'", "")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), "Adding alias for.*igrep")

	expected := `aliases:
    igrep: '!gh issue list | grep'
`
	assert.Equal(t, expected, mainBuf.String())
}

func TestShellAlias_bang(t *testing.T) {
	mainBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, ioutil.Discard)()

	cfg := config.NewFromString(``)

	output, err := runCommand(cfg, true, "igrep '!gh issue list | grep'", "")
	require.NoError(t, err)

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), "Adding alias for.*igrep")

	expected := `aliases:
    igrep: '!gh issue list | grep'
`
	assert.Equal(t, expected, mainBuf.String())
}

func TestShellAlias_from_stdin(t *testing.T) {
	mainBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, ioutil.Discard)()

	cfg := config.NewFromString(``)

	output, err := runCommand(cfg, true, "users -", `api graphql -F name="$1" -f query='
    query ($name: String!) {
        user(login: $name) {
            name
        }
    }'`)

	require.NoError(t, err)

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), "Adding alias for.*users")

	expected := `aliases:
    users: |-
        api graphql -F name="$1" -f query='
            query ($name: String!) {
                user(login: $name) {
                    name
                }
            }'
`

	assert.Equal(t, expected, mainBuf.String())
}

func TestShellAlias_getExpansion(t *testing.T) {
	tests := []struct {
		name         string
		want         string
		expansionArg string
		stdin        string
	}{
		{
			name:         "co",
			want:         "pr checkout",
			expansionArg: "pr checkout",
		},
		{
			name:         "co",
			want:         "pr checkout",
			expansionArg: "pr checkout",
			stdin:        "api graphql -F name=\"$1\"",
		},
		{
			name:         "stdin",
			expansionArg: "-",
			want:         "api graphql -F name=\"$1\"",
			stdin:        "api graphql -F name=\"$1\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, stdin, _, _ := iostreams.Test()

			io.SetStdinTTY(false)

			_, err := stdin.WriteString(tt.stdin)
			assert.NoError(t, err)

			expansion, err := getExpansion(&SetOptions{
				Expansion: tt.expansionArg,
				IO:        io,
			})
			assert.NoError(t, err)

			assert.Equal(t, expansion, tt.want)
		})
	}
}
