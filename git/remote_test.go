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
	assert.Equal(t, 4, len(r))

	assert.Equal(t, "mona", r[0].Name)
	assert.Equal(t, "ssh://git@github.com/monalisa/myfork.git", r[0].FetchURL.String())
	if r[0].PushURL != nil {
		t.Errorf("expected no PushURL, got %q", r[0].PushURL)
	}
	assert.Equal(t, "origin", r[1].Name)
	assert.Equal(t, "/monalisa/octo-cat.git", r[1].FetchURL.Path)
	assert.Equal(t, "/monalisa/octo-cat-push.git", r[1].PushURL.Path)

	assert.Equal(t, "upstream", r[2].Name)
	assert.Equal(t, "example.com", r[2].FetchURL.Host)
	assert.Equal(t, "github.com", r[2].PushURL.Host)

	assert.Equal(t, "zardoz", r[3].Name)
}
