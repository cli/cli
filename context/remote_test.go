package context

import (
	"net/url"
	"testing"

	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/stretchr/testify/assert"
)

func Test_Remotes_FindByName(t *testing.T) {
	list := Remotes{
		&Remote{Remote: &git.Remote{Name: "mona"}, Repo: ghrepo.New("monalisa", "myfork")},
		&Remote{Remote: &git.Remote{Name: "origin"}, Repo: ghrepo.New("monalisa", "octo-cat")},
		&Remote{Remote: &git.Remote{Name: "upstream"}, Repo: ghrepo.New("hubot", "tools")},
	}

	r, err := list.FindByName("upstream", "origin")
	assert.NoError(t, err)
	assert.Equal(t, "upstream", r.Name)

	r, err = list.FindByName("nonexistent", "*")
	assert.NoError(t, err)
	assert.Equal(t, "mona", r.Name)

	_, err = list.FindByName("nonexistent")
	assert.Error(t, err, "no GitHub remotes found")
}

func Test_translateRemotes(t *testing.T) {
	publicURL, _ := url.Parse("https://github.com/monalisa/hello")
	originURL, _ := url.Parse("http://example.com/repo")

	gitRemotes := git.RemoteSet{
		&git.Remote{
			Name:     "origin",
			FetchURL: originURL,
		},
		&git.Remote{
			Name:     "public",
			FetchURL: publicURL,
		},
	}

	identityURL := func(u *url.URL) *url.URL {
		return u
	}
	result := TranslateRemotes(gitRemotes, identityURL)

	if len(result) != 1 {
		t.Errorf("got %d results", len(result))
	}
	if result[0].Name != "public" {
		t.Errorf("got %q", result[0].Name)
	}
	if result[0].RepoName() != "hello" {
		t.Errorf("got %q", result[0].RepoName())
	}
}

func Test_FilterByHosts(t *testing.T) {
	r1 := &Remote{Remote: &git.Remote{Name: "mona"}, Repo: ghrepo.NewWithHost("monalisa", "myfork", "test.com")}
	r2 := &Remote{Remote: &git.Remote{Name: "origin"}, Repo: ghrepo.NewWithHost("monalisa", "octo-cat", "example.com")}
	r3 := &Remote{Remote: &git.Remote{Name: "upstream"}, Repo: ghrepo.New("hubot", "tools")}
	list := Remotes{r1, r2, r3}
	f := list.FilterByHosts([]string{"example.com", "test.com"})
	assert.Equal(t, 2, len(f))
	assert.Equal(t, r1, f[0])
	assert.Equal(t, r2, f[1])
}
