package context

import (
	"testing"
)

func Test_repoFromURL(t *testing.T) {
	r, err := repoFromURL("http://github.com/monalisa/octo-cat.git")
	if err != nil {
		t.Error(err)
	}
	if r.Owner != "monalisa" {
		t.Errorf("got Owner: %q", r.Owner)
	}
	if r.Name != "octo-cat" {
		t.Errorf("got Name: %q", r.Name)
	}
}
