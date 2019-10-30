package git

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// TODO: extract assertion helpers into a shared package
func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}

func Test_sshParse(t *testing.T) {
	m := sshParse(strings.NewReader(`
	Host foo bar
		HostName example.com
	`), strings.NewReader(`
	Host bar baz
	hostname %%%h.net%%
	`))
	eq(t, m["foo"], "example.com")
	eq(t, m["bar"], "%bar.net%")
	eq(t, m["nonexist"], "")
}

func Test_Translator(t *testing.T) {
	m := SSHAliasMap{
		"gh":         "github.com",
		"github.com": "ssh.github.com",
	}
	tr := m.Translator()

	cases := [][]string{
		[]string{"ssh://gh/o/r", "ssh://github.com/o/r"},
		[]string{"ssh://github.com/o/r", "ssh://github.com/o/r"},
		[]string{"https://gh/o/r", "https://gh/o/r"},
	}
	for _, c := range cases {
		u, _ := url.Parse(c[0])
		got := tr(u)
		if got.String() != c[1] {
			t.Errorf("%q: expected %q, got %q", c[0], c[1], got)
		}
	}
}
