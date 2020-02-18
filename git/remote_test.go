package git

import (
	"testing"

	"github.com/cli/cli/internal"
)

func Test_parseRemotes(t *testing.T) {
	remoteList := []string{
		"mona\tgit@" + internal.Host + ":monalisa/myfork.git (fetch)",
		"origin\thttps://" + internal.Host + "/monalisa/octo-cat.git (fetch)",
		"origin\thttps://" + internal.Host + "/monalisa/octo-cat-push.git (push)",
		"upstream\thttps://example.com/nowhere.git (fetch)",
		"upstream\thttps://" + internal.Host + "/hubot/tools (push)",
		"zardoz\thttps://example.com/zed.git (push)",
	}
	r := parseRemotes(remoteList)
	eq(t, len(r), 4)

	eq(t, r[0].Name, "mona")
	eq(t, r[0].FetchURL.String(), "ssh://git@"+internal.Host+"/monalisa/myfork.git")
	if r[0].PushURL != nil {
		t.Errorf("expected no PushURL, got %q", r[0].PushURL)
	}
	eq(t, r[1].Name, "origin")
	eq(t, r[1].FetchURL.Path, "/monalisa/octo-cat.git")
	eq(t, r[1].PushURL.Path, "/monalisa/octo-cat-push.git")

	eq(t, r[2].Name, "upstream")
	eq(t, r[2].FetchURL.Host, "example.com")
	eq(t, r[2].PushURL.Host, internal.Host)

	eq(t, r[3].Name, "zardoz")
}
