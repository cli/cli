package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.Equal(t, len(r), 4)

	assert.Equal(t, r[0].Name, "mona")
	assert.Equal(t, r[0].FetchURL.String(), "ssh://git@github.com/monalisa/myfork.git")
	if r[0].PushURL != nil {
		t.Errorf("expected no PushURL, got %q", r[0].PushURL)
	}
	assert.Equal(t, r[1].Name, "origin")
	assert.Equal(t, r[1].FetchURL.Path, "/monalisa/octo-cat.git")
	assert.Equal(t, r[1].PushURL.Path, "/monalisa/octo-cat-push.git")

	assert.Equal(t, r[2].Name, "upstream")
	assert.Equal(t, r[2].FetchURL.Host, "example.com")
	assert.Equal(t, r[2].PushURL.Host, "github.com")

	assert.Equal(t, r[3].Name, "zardoz")
}
