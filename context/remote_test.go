package context

import (
	"testing"
)

func Test_repoFromURL(t *testing.T) {
	r, err := repoFromURL("http://github.com/monalisa/octo-cat.git")
	eq(t, err, nil)
	eq(t, r, &GitHubRepository{Owner: "monalisa", Name: "octo-cat"})
}

func Test_parseRemotes(t *testing.T) {
	remoteList := []string{
		"mona\tgit@github.com:monalisa/myfork.git (fetch)",
		"origin\thttps://github.com/monalisa/octo-cat.git (fetch)",
		"origin\thttps://github.com/monalisa/octo-cat-push.git (push)",
		"upstream\thttps://example.com/nowhere.git (fetch)",
		"upstream\thttps://github.com/hubot/tools (push)",
	}
	r := parseRemotes(remoteList)
	eq(t, len(r), 3)

	eq(t, r[0], &Remote{Name: "mona", Owner: "monalisa", Repo: "myfork"})
	eq(t, r[1], &Remote{Name: "origin", Owner: "monalisa", Repo: "octo-cat"})
	eq(t, r[2], &Remote{Name: "upstream", Owner: "hubot", Repo: "tools"})
}
