package context

import (
	"errors"
	"net/url"
	"testing"

	"github.com/github/gh-cli/git"
)

func Test_repoFromURL(t *testing.T) {
	u, _ := url.Parse("http://github.com/monalisa/octo-cat.git")
	owner, repo, err := repoFromURL(u)
	eq(t, err, nil)
	eq(t, owner, "monalisa")
	eq(t, repo, "octo-cat")
}

func Test_repoFromURL_invalid(t *testing.T) {
	cases := [][]string{
		[]string{
			"https://example.com/one/two",
			"unsupported hostname: example.com",
		},
		[]string{
			"/path/to/disk",
			"unsupported hostname: ",
		},
	}
	for _, c := range cases {
		u, _ := url.Parse(c[0])
		_, _, err := repoFromURL(u)
		eq(t, err, errors.New(c[1]))
	}
}

func Test_Remotes_FindByName(t *testing.T) {
	list := Remotes{
		&Remote{Remote: &git.Remote{Name: "mona"}, Owner: "monalisa", Repo: "myfork"},
		&Remote{Remote: &git.Remote{Name: "origin"}, Owner: "monalisa", Repo: "octo-cat"},
		&Remote{Remote: &git.Remote{Name: "upstream"}, Owner: "hubot", Repo: "tools"},
	}

	r, err := list.FindByName("upstream", "origin")
	eq(t, err, nil)
	eq(t, r.Name, "upstream")

	r, err = list.FindByName("nonexist", "*")
	eq(t, err, nil)
	eq(t, r.Name, "mona")

	_, err = list.FindByName("nonexist")
	eq(t, err, errors.New(`no GitHub remotes found`))
}
