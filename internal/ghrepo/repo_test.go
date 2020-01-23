package ghrepo

import (
	"net/url"
	"testing"
)

func Test_repoFromURL(t *testing.T) {
	u, _ := url.Parse("http://github.com/monalisa/octo-cat.git")
	repo, err := FromURL(u)
	if err != nil {
		t.Fatalf("got error %q", err)
	}
	if repo.RepoOwner() != "monalisa" {
		t.Errorf("got owner %q", repo.RepoOwner())
	}
	if repo.RepoName() != "octo-cat" {
		t.Errorf("got name %q", repo.RepoName())
	}
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
		_, err := FromURL(u)
		if err == nil || err.Error() != c[1] {
			t.Errorf("got %q", err)
		}
	}
}
