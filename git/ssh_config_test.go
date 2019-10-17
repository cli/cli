package git

import (
	"reflect"
	"strings"
	"testing"
)

// TODO: extract assertion helpers into a shared package
func eq(t *testing.T, got interface{}, expected interface{}) {
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
