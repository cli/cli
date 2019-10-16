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

func Test_parseRemotes(t *testing.T) {
	remoteList := []string{
		"mona\tgit@github.com:monalisa/myfork.git (fetch)",
		"origin\thttps://github.com/monalisa/octo-cat.git (fetch)",
		"origin\thttps://github.com/monalisa/octo-cat-push.git (push)",
		"upstream\thttps://example.com/nowhere.git (fetch)",
		"upstream\thttps://github.com/hubot/tools (push)",
	}
	r, err := parseRemotes(remoteList)
	if err != nil {
		t.Error(err)
	}

	mona, err := r.FindByName("mona")
	if err != nil {
		t.Error(err)
	}
	if mona.Owner != "monalisa" || mona.Repo != "myfork" {
		t.Errorf("got %s/%s", mona.Owner, mona.Repo)
	}

	origin, err := r.FindByName("origin")
	if err != nil {
		t.Error(err)
	}
	if origin.Owner != "monalisa" || origin.Repo != "octo-cat" {
		t.Errorf("got %s/%s", origin.Owner, origin.Repo)
	}

	upstream, err := r.FindByName("upstream")
	if err != nil {
		t.Error(err)
	}
	if upstream.Owner != "hubot" || upstream.Repo != "tools" {
		t.Errorf("got %s/%s", upstream.Owner, upstream.Repo)
	}
}
