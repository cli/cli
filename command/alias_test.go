package command

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func stubSh(value string) func() {
	orig := findSh
	findSh = func() (string, error) {
		return value, nil
	}
	return func() {
		findSh = orig
	}
}

func TestExpandAlias_shell(t *testing.T) {
	defer stubSh("sh")()
	cfg := `---
aliases:
  ig: '!gh issue list | grep cool'
`
	initBlankContext(cfg, "OWNER/REPO", "trunk")

	expanded, isShell, err := ExpandAlias([]string{"gh", "ig"})

	assert.True(t, isShell)

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	expected := []string{"sh", "-c", "gh issue list | grep cool"}

	assert.Equal(t, expected, expanded)
}

func TestExpandAlias_shell_extra_args(t *testing.T) {
	defer stubSh("sh")()
	cfg := `---
aliases:
  ig: '!gh issue list --label=$1 | grep'
`
	initBlankContext(cfg, "OWNER/REPO", "trunk")

	expanded, isShell, err := ExpandAlias([]string{"gh", "ig", "bug", "foo"})

	assert.True(t, isShell)

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	expected := []string{"sh", "-c", "gh issue list --label=$1 | grep", "--", "bug", "foo"}

	assert.Equal(t, expected, expanded)
}

func TestExpandAlias(t *testing.T) {
	cfg := `---
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

		expanded, isShell, err := ExpandAlias(args)

		assert.False(t, isShell)

		if err == nil && c.Err != "" {
			t.Errorf("expected error %s for %s", c.Err, c.Args)
			continue
		}

		if err != nil {
			eq(t, err.Error(), c.Err)
			continue
		}

		assert.Equal(t, c.ExpectedArgs, expanded)
	}
}
