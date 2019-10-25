package context

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}

func Test_parseConfig(t *testing.T) {
	c := strings.NewReader(`---
github.com:
- user: monalisa
  oauth_token: OTOKEN
  protocol: https
- user: wronguser
  oauth_token: NOTTHIS
`)
	entry, err := parseConfig(c)
	eq(t, err, nil)
	eq(t, entry.User, "monalisa")
	eq(t, entry.Token, "OTOKEN")
}

func Test_parseConfig_multipleHosts(t *testing.T) {
	c := strings.NewReader(`---
example.com:
- user: wronguser
  oauth_token: NOTTHIS
github.com:
- user: monalisa
  oauth_token: OTOKEN
`)
	entry, err := parseConfig(c)
	eq(t, err, nil)
	eq(t, entry.User, "monalisa")
	eq(t, entry.Token, "OTOKEN")
}

func Test_parseConfig_notFound(t *testing.T) {
	c := strings.NewReader(`---
example.com:
- user: wronguser
  oauth_token: NOTTHIS
`)
	_, err := parseConfig(c)
	eq(t, err, errors.New(`could not find config entry for "github.com"`))
}
