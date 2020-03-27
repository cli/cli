package ghrepo

import (
	"fmt"
	"net/url"
	"strings"
)

const defaultHostname = "github.com"

type Interface interface {
	RepoName() string
	RepoOwner() string
}

func New(owner, repo string) Interface {
	return &ghRepo{
		owner: owner,
		name:  repo,
	}
}
func FullName(r Interface) string {
	return fmt.Sprintf("%s/%s", r.RepoOwner(), r.RepoName())
}

func FromFullName(nwo string) Interface {
	var r ghRepo
	parts := strings.SplitN(nwo, "/", 2)
	if len(parts) == 2 {
		r.owner, r.name = parts[0], parts[1]
	}
	return &r
}

func FromURL(u *url.URL) (Interface, error) {
	if !strings.EqualFold(u.Hostname(), defaultHostname) && !strings.EqualFold(u.Hostname(), "www."+defaultHostname) {
		return nil, fmt.Errorf("unsupported hostname: %s", u.Hostname())
	}
	parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid path: %s", u.Path)
	}
	return New(parts[0], strings.TrimSuffix(parts[1], ".git")), nil
}

func IsSame(a, b Interface) bool {
	return strings.EqualFold(a.RepoOwner(), b.RepoOwner()) &&
		strings.EqualFold(a.RepoName(), b.RepoName())
}

type ghRepo struct {
	owner string
	name  string
}

func (r ghRepo) RepoOwner() string {
	return r.owner
}
func (r ghRepo) RepoName() string {
	return r.name
}
