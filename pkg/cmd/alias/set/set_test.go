package set

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runCommand(cfg config.Config, isTTY bool, cli string) (*test.CmdOut, error) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(isTTY)
	io.SetStdinTTY(isTTY)
	io.SetStderrTTY(isTTY)

	factory := &cmdutil.Factory{
		IOStreams: io,
		Config: func() (config.Config, error) {
			return cfg, nil
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

	argv, err := shlex.Split("set " + cli)
	if err != nil {
		return nil, err
	}
	rootCmd.SetArgs(argv)

	rootCmd.SetIn(&bytes.Buffer{})
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

	_, err := runCommand(cfg, true, "pr 'pr status'")

	if assert.Error(t, err) {
		assert.Equal(t, `could not create alias: "pr" is already a gh command`, err.Error())
	}
}

func TestAliasSet_empty_aliases(t *testing.T) {
	mainBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, ioutil.Discard)()

	cfg := config.NewFromString(heredoc.Doc(`
		aliases:
		editor: vim
	`))

	output, err := runCommand(cfg, true, "co 'pr checkout'")

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	test.ExpectLines(t, output.Stderr(), "Added alias")
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

	output, err := runCommand(cfg, true, "co 'pr checkout -Rcool/repo'")
	require.NoError(t, err)

	test.ExpectLines(t, output.Stderr(), "Changed alias.*co.*from.*pr checkout.*to.*pr checkout -Rcool/repo")
}

func TestAliasSet_space_args(t *testing.T) {
	mainBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, ioutil.Discard)()

	cfg := config.NewFromString(``)

	output, err := runCommand(cfg, true, `il 'issue list -l "cool story"'`)
	require.NoError(t, err)

	test.ExpectLines(t, output.Stderr(), `Adding alias for.*il.*issue list -l "cool story"`)

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

			output, err := runCommand(cfg, true, c.Cmd)
			if err != nil {
				t.Fatalf("got unexpected error running %s: %s", c.Cmd, err)
			}

			test.ExpectLines(t, output.Stderr(), c.ExpectedOutputLine)
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

	output, err := runCommand(cfg, true, "diff 'pr diff'")
	require.NoError(t, err)

	expected := `editor: vim
aliases:
    diff: pr diff
`

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

	output, err := runCommand(cfg, true, "view 'pr view'")
	require.NoError(t, err)

	expected := `aliases:
    foo: bar
    view: pr view
`

	test.ExpectLines(t, output.Stderr(), "Adding alias for.*view.*pr view", "Added alias.")
	assert.Equal(t, expected, mainBuf.String())

}

func TestAliasSet_invalid_command(t *testing.T) {
	defer config.StubWriteConfig(ioutil.Discard, ioutil.Discard)()

	cfg := config.NewFromString(``)

	_, err := runCommand(cfg, true, "co 'pe checkout'")
	if assert.Error(t, err) {
		assert.Equal(t, "could not create alias: pe checkout does not correspond to a gh command", err.Error())
	}
}

func TestShellAlias_flag(t *testing.T) {
	mainBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, ioutil.Discard)()

	cfg := config.NewFromString(``)

	output, err := runCommand(cfg, true, "--shell igrep 'gh issue list | grep'")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

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

	output, err := runCommand(cfg, true, "igrep '!gh issue list | grep'")
	require.NoError(t, err)

	test.ExpectLines(t, output.Stderr(), "Adding alias for.*igrep")

	expected := `aliases:
    igrep: '!gh issue list | grep'
`
	assert.Equal(t, expected, mainBuf.String())
}
