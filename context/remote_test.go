package context

import (
	"errors"
	"testing"

	"github.com/cli/cli/git"
)

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
