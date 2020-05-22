package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/test"
)

func TestAliasSet_gh_command(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "trunk")

	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

	_, err := RunCommand("alias set pr pr status")
	if err == nil {
		t.Fatal("expected error")
	}

	eq(t, err.Error(), `could not create alias: "pr" is already a gh command`)
}

func TestAliasSet_empty_aliases(t *testing.T) {
	cfg := `---
aliases:
editor: vim
`
	initBlankContext(cfg, "OWNER/REPO", "trunk")

	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

	output, err := RunCommand("alias set co pr checkout")

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	test.ExpectLines(t, output.String(), "Added alias")

	expected := `aliases:
    co: pr checkout
editor: vim
`
	eq(t, mainBuf.String(), expected)
}

func TestAliasSet_existing_alias(t *testing.T) {
	cfg := `---
hosts:
  github.com:
    user: OWNER
    oauth_token: token123
aliases:
  co: pr checkout
`
	initBlankContext(cfg, "OWNER/REPO", "trunk")

	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

	output, err := RunCommand("alias set co pr checkout -Rcool/repo")

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	test.ExpectLines(t, output.String(), "Changed alias co from pr checkout to pr checkout -Rcool/repo")
}

func TestAliasSet_space_args(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "trunk")

	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

	output, err := RunCommand(`alias set il issue list -l 'cool story'`)

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	test.ExpectLines(t, output.String(), `Adding alias for il: issue list -l "cool story"`)

	test.ExpectLines(t, mainBuf.String(), `il: issue list -l "cool story"`)
}

func TestAliasSet_arg_processing(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "trunk")
	cases := []struct {
		Cmd                string
		ExpectedOutputLine string
		ExpectedConfigLine string
	}{
		{"alias set co pr checkout", "- Adding alias for co: pr checkout", "co: pr checkout"},

		{`alias set il "issue list"`, "- Adding alias for il: issue list", "il: issue list"},

		{`alias set iz 'issue list'`, "- Adding alias for iz: issue list", "iz: issue list"},

		{`alias set iy issue list --author=\$1 --label=\$2`,
			`- Adding alias for iy: issue list --author=\$1 --label=\$2`,
			`iy: issue list --author=\$1 --label=\$2`},

		{`alias set ii 'issue list --author="$1" --label="$2"'`,
			`- Adding alias for ii: issue list --author=\$1 --label=\$2`,
			`ii: issue list --author=\$1 --label=\$2`},

		{`alias set ix issue list --author='$1' --label='$2'`,
			`- Adding alias for ix: issue list --author=\$1 --label=\$2`,
			`ix: issue list --author=\$1 --label=\$2`},
	}

	for _, c := range cases {
		mainBuf := bytes.Buffer{}
		hostsBuf := bytes.Buffer{}
		defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

		output, err := RunCommand(c.Cmd)
		if err != nil {
			t.Fatalf("got unexpected error running %s: %s", c.Cmd, err)
		}

		test.ExpectLines(t, output.String(), c.ExpectedOutputLine)
		test.ExpectLines(t, mainBuf.String(), c.ExpectedConfigLine)
	}
}

func TestAliasSet_init_alias_cfg(t *testing.T) {
	cfg := `---
editor: vim
`
	initBlankContext(cfg, "OWNER/REPO", "trunk")

	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

	output, err := RunCommand("alias set diff pr diff")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	expected := `editor: vim
aliases:
    diff: pr diff
`

	test.ExpectLines(t, output.String(), "Adding alias for diff: pr diff", "Added alias.")
	eq(t, mainBuf.String(), expected)
}

func TestAliasSet_existing_aliases(t *testing.T) {
	cfg := `---
aliases:
    foo: bar
`
	initBlankContext(cfg, "OWNER/REPO", "trunk")

	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

	output, err := RunCommand("alias set view pr view")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	expected := `aliases:
    foo: bar
    view: pr view
`

	test.ExpectLines(t, output.String(), "Adding alias for view: pr view", "Added alias.")
	eq(t, mainBuf.String(), expected)

}

func TestExpandAlias(t *testing.T) {
	cfg := `---
hosts:
  github.com:
    user: OWNER
    oauth_token: token123
aliases:
  co: pr checkout
  il: issue list --author="$1" --label="$2"
  ia: issue list --author="$1" --assignee="$1"
`
	initBlankContext(cfg, "OWNER/REPO", "trunk")
	for _, c := range []struct {
		Args         string
		ExpectedArgs []string
		Err          string
	}{
		{"gh co", []string{"pr", "checkout"}, ""},
		{"gh il", nil, `not enough arguments for alias: issue list --author="$1" --label="$2"`},
		{"gh il vilmibm", nil, `not enough arguments for alias: issue list --author="vilmibm" --label="$2"`},
		{"gh co 123", []string{"pr", "checkout", "123"}, ""},
		{"gh il vilmibm epic", []string{"issue", "list", `--author=vilmibm`, `--label=epic`}, ""},
		{"gh ia vilmibm", []string{"issue", "list", `--author=vilmibm`, `--assignee=vilmibm`}, ""},
		{"gh ia $coolmoney$", []string{"issue", "list", `--author=$coolmoney$`, `--assignee=$coolmoney$`}, ""},
		{"gh pr status", []string{"pr", "status"}, ""},
		{"gh il vilmibm epic -R vilmibm/testing", []string{"issue", "list", "--author=vilmibm", "--label=epic", "-R", "vilmibm/testing"}, ""},
		{"gh dne", []string{"dne"}, ""},
		{"gh", []string{}, ""},
		{"", []string{}, ""},
	} {
		args := []string{}
		if c.Args != "" {
			args = strings.Split(c.Args, " ")
		}

		out, err := ExpandAlias(args)

		if err == nil && c.Err != "" {
			t.Errorf("expected error %s for %s", c.Err, c.Args)
			continue
		}

		if err != nil {
			eq(t, err.Error(), c.Err)
			continue
		}

		eq(t, out, c.ExpectedArgs)
	}
}

func TestAliasSet_invalid_command(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "trunk")
	_, err := RunCommand("alias set co pe checkout")
	if err == nil {
		t.Fatal("expected error")
	}

	eq(t, err.Error(), "could not create alias: pe checkout does not correspond to a gh command")
}

func TestAliasList_empty(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "trunk")

	output, err := RunCommand("alias list")

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	eq(t, output.String(), "")
}

func TestAliasList(t *testing.T) {
	cfg := `---
hosts:
  github.com:
    user: OWNER
    oauth_token: token123
aliases:
  co: pr checkout
  il: issue list --author=$1 --label=$2
  clone: repo clone
  prs: pr status
  cs: config set editor 'quoted path'
`
	initBlankContext(cfg, "OWNER/REPO", "trunk")

	output, err := RunCommand("alias list")

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	test.ExpectLines(t, output.String(),
		"clone:\trepo clone",
		"co:\tpr checkout",
		"il:\tissue list --author=\\$1 --label=\\$2",
		"prs:\tpr status",
		"cs:\tconfig set editor 'quoted path'")
}
