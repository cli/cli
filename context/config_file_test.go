package context

import (
	"strings"
	"testing"
)

func Test_parseConfig(t *testing.T) {
	c := strings.NewReader(`---
github.com:
- user: monalisa
  oauth_token: OTOKEN
`)
	entry, err := parseConfig(c)
	if err != nil {
		t.Error(err)
	}
	if entry.User != "monalisa" {
		t.Errorf("got User: %q", entry.User)
	}
	if entry.Token != "OTOKEN" {
		t.Errorf("got User: %q", entry.Token)
	}
}
