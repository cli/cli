package git

import (
	"testing"

	"github.com/cli/cli/test"
)

func Test_parseRemotes(t *testing.T) {
	remoteList := []string{
		"mona\tgit@github.com:monalisa/myfork.git (fetch)",
		"origin\thttps://github.com/monalisa/octo-cat.git (fetch)",
		"origin\thttps://github.com/monalisa/octo-cat-push.git (push)",
		"upstream\thttps://example.com/nowhere.git (fetch)",
		"upstream\thttps://github.com/hubot/tools (push)",
		"zardoz\thttps://example.com/zed.git (push)",
	}
	r := parseRemotes(remoteList)
	test.Eq(t, len(r), 4)

	test.Eq(t, r[0].Name, "mona")
	test.Eq(t, r[0].FetchURL.String(), "ssh://git@github.com/monalisa/myfork.git")
	if r[0].PushURL != nil {
		t.Errorf("expected no PushURL, got %q", r[0].PushURL)
	}
	test.Eq(t, r[1].Name, "origin")
	test.Eq(t, r[1].FetchURL.Path, "/monalisa/octo-cat.git")
	test.Eq(t, r[1].PushURL.Path, "/monalisa/octo-cat-push.git")

	test.Eq(t, r[2].Name, "upstream")
	test.Eq(t, r[2].FetchURL.Host, "example.com")
	test.Eq(t, r[2].PushURL.Host, "github.com")

	test.Eq(t, r[3].Name, "zardoz")
}
