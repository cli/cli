package context

import (
	"errors"
	"testing"

	"github.com/github/gh-cli/git"
)

func Test_repoFromURL(t *testing.T) {
	git.InitSSHAliasMap(nil)

	r, err := repoFromURL("http://github.com/monalisa/octo-cat.git")
	eq(t, err, nil)
	eq(t, r, &GitHubRepository{Owner: "monalisa", Name: "octo-cat"})
}

func Test_repoFromURL_invalid(t *testing.T) {
	git.InitSSHAliasMap(nil)

	_, err := repoFromURL("https://example.com/one/two")
	eq(t, err, errors.New(`invalid hostname: example.com`))

	_, err = repoFromURL("/path/to/disk")
	eq(t, err, errors.New(`invalid hostname: `))
}

func Test_repoFromURL_SSH(t *testing.T) {
	git.InitSSHAliasMap(map[string]string{
		"gh":         "github.com",
		"github.com": "ssh.github.com",
	})

	r, err := repoFromURL("git@gh:monalisa/octo-cat")
	eq(t, err, nil)
	eq(t, r, &GitHubRepository{Owner: "monalisa", Name: "octo-cat"})

	r, err = repoFromURL("git@github.com:monalisa/octo-cat")
	eq(t, err, nil)
	eq(t, r, &GitHubRepository{Owner: "monalisa", Name: "octo-cat"})
}

func Test_parseRemotes(t *testing.T) {
	git.InitSSHAliasMap(nil)

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
