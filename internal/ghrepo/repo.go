package ghrepo

import (
	"fmt"
	"net/url"
	"strings"
)

// TODO these are sprinkled across command, context, config, and ghrepo
const defaultHostname = "github.com"

// Interface describes an object that represents a GitHub repository
type Interface interface {
	RepoName() string
	RepoOwner() string
}

// New instantiates a GitHub repository from owner and name arguments
func New(owner, repo string) Interface {
	return &ghRepo{
		owner: owner,
		name:  repo,
	}
}

// FullName serializes a GitHub repository into an "OWNER/REPO" string
func FullName(r Interface) string {
	return fmt.Sprintf("%s/%s", r.RepoOwner(), r.RepoName())
}

// FromFullName extracts the GitHub repository inforation from an "OWNER/REPO" string
func FromFullName(nwo string) Interface {
	var r ghRepo
	parts := strings.SplitN(nwo, "/", 2)
	if len(parts) == 2 {
		r.owner, r.name = parts[0], parts[1]
	}
	return &r
}

// FromURL extracts the GitHub repository information from a URL
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

// IsSame compares two GitHub repositories
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
