package ghrepo

import (
	"fmt"
	"net/url"
	"strings"
)

const defaultHostname = "github.com"

// Interface describes an object that represents a GitHub repository
type Interface interface {
	RepoName() string
	RepoOwner() string
	RepoHost() string
}

// New instantiates a GitHub repository from owner and name arguments
func New(owner, repo string) Interface {
	return &ghRepo{
		owner: owner,
		name:  repo,
	}
}

// NewWithHost is like New with an explicit host name
func NewWithHost(owner, repo, hostname string) Interface {
	return &ghRepo{
		owner:    owner,
		name:     repo,
		hostname: hostname,
	}
}

// FullName serializes a GitHub repository into an "OWNER/REPO" string
func FullName(r Interface) string {
	return fmt.Sprintf("%s/%s", r.RepoOwner(), r.RepoName())
}

// FromFullName extracts the GitHub repository information from an "OWNER/REPO" string
func FromFullName(nwo string) (Interface, error) {
	var r ghRepo
	parts := strings.SplitN(nwo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return &r, fmt.Errorf("expected OWNER/REPO format, got %q", nwo)
	}
	r.owner, r.name = parts[0], parts[1]
	return &r, nil
}

// FromURL extracts the GitHub repository information from a git remote URL
func FromURL(u *url.URL) (Interface, error) {
	if u.Hostname() == "" {
		return nil, fmt.Errorf("no hostname detected")
	}

	parts := strings.SplitN(strings.Trim(u.Path, "/"), "/", 3)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid path: %s", u.Path)
	}

	return &ghRepo{
		owner:    parts[0],
		name:     strings.TrimSuffix(parts[1], ".git"),
		hostname: normalizeHostname(u.Hostname()),
	}, nil
}

func normalizeHostname(h string) string {
	return strings.ToLower(strings.TrimPrefix(h, "www."))
}

// IsSame compares two GitHub repositories
func IsSame(a, b Interface) bool {
	return strings.EqualFold(a.RepoOwner(), b.RepoOwner()) &&
		strings.EqualFold(a.RepoName(), b.RepoName()) &&
		normalizeHostname(a.RepoHost()) == normalizeHostname(b.RepoHost())
}

func GenerateRepoURL(repo Interface, p string, args ...interface{}) string {
	baseURL := fmt.Sprintf("https://%s/%s/%s", repo.RepoHost(), repo.RepoOwner(), repo.RepoName())
	if p != "" {
		return baseURL + "/" + fmt.Sprintf(p, args...)
	}
	return baseURL
}

type ghRepo struct {
	owner    string
	name     string
	hostname string
}

func (r ghRepo) RepoOwner() string {
	return r.owner
}

func (r ghRepo) RepoName() string {
	return r.name
}

func (r ghRepo) RepoHost() string {
	if r.hostname != "" {
		return r.hostname
	}
	return defaultHostname
}
