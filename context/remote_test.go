package context

import (
	"net/url"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
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

func Test_Remotes_FindByRepo(t *testing.T) {
	list := Remotes{
		&Remote{Remote: &git.Remote{Name: "remote-0"}, Repo: ghrepo.New("owner", "repo")},
		&Remote{Remote: &git.Remote{Name: "remote-1"}, Repo: ghrepo.New("another-owner", "another-repo")},
	}

	tests := []struct {
		name        string
		owner       string
		repo        string
		wantsRemote *Remote
		wantsError  string
	}{
		{
			name:        "exact match (owner/repo)",
			owner:       "owner",
			repo:        "repo",
			wantsRemote: list[0],
		},
		{
			name:        "exact match (another-owner/another-repo)",
			owner:       "another-owner",
			repo:        "another-repo",
			wantsRemote: list[1],
		},
		{
			name:        "case-insensitive match",
			owner:       "OWNER",
			repo:        "REPO",
			wantsRemote: list[0],
		},
		{
			name:       "non-match (owner)",
			owner:      "unknown-owner",
			repo:       "repo",
			wantsError: "no matching remote found; looking for unknown-owner/repo",
		},
		{
			name:       "non-match (repo)",
			owner:      "owner",
			repo:       "unknown-repo",
			wantsError: "no matching remote found; looking for owner/unknown-repo",
		},
		{
			name:       "non-match (owner, repo)",
			owner:      "unknown-owner",
			repo:       "unknown-repo",
			wantsError: "no matching remote found; looking for unknown-owner/unknown-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := list.FindByRepo(tt.owner, tt.repo)
			if tt.wantsError != "" {
				assert.Error(t, err, tt.wantsError)
				assert.Nil(t, r)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, r, tt.wantsRemote)
			}
		})
	}
}

type identityTranslator struct{}

func (it identityTranslator) Translate(u *url.URL) *url.URL {
	return u
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

	result := TranslateRemotes(gitRemotes, identityTranslator{})

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

func Test_Remotes_RepoContext(t *testing.T) {
	t.Run("returns false for empty", func(t *testing.T) {
		repo, ok := Remotes{}.RepoContext()
		assert.False(t, ok)
		assert.Nil(t, repo)
	})

	t.Run("prefers resolved base remote", func(t *testing.T) {
		list := Remotes{
			&Remote{Remote: &git.Remote{Name: "origin", Resolved: ""}, Repo: ghrepo.NewWithHost("owner", "repo", "github.com")},
			&Remote{Remote: &git.Remote{Name: "upstream", Resolved: "base"}, Repo: ghrepo.NewWithHost("base-owner", "base-repo", "github.com")},
		}

		repo, ok := list.RepoContext()
		assert.True(t, ok)
		assert.Equal(t, "base-owner", repo.RepoOwner())
		assert.Equal(t, "base-repo", repo.RepoName())
	})

	t.Run("uses explicit resolved repo", func(t *testing.T) {
		list := Remotes{
			&Remote{Remote: &git.Remote{Name: "origin", Resolved: "other-owner/other-repo"}, Repo: ghrepo.NewWithHost("owner", "repo", "github.com")},
		}

		repo, ok := list.RepoContext()
		assert.True(t, ok)
		assert.Equal(t, "other-owner", repo.RepoOwner())
		assert.Equal(t, "other-repo", repo.RepoName())
		assert.Equal(t, "github.com", repo.RepoHost())
	})

	t.Run("falls back to first remote", func(t *testing.T) {
		list := Remotes{
			&Remote{Remote: &git.Remote{Name: "origin", Resolved: ""}, Repo: ghrepo.NewWithHost("owner", "repo", "github.com")},
			&Remote{Remote: &git.Remote{Name: "fork", Resolved: ""}, Repo: ghrepo.NewWithHost("other", "repo", "github.com")},
		}

		repo, ok := list.RepoContext()
		assert.True(t, ok)
		assert.Equal(t, "owner", repo.RepoOwner())
		assert.Equal(t, "repo", repo.RepoName())
	})
}
