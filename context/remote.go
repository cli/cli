package context

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/github/gh-cli/git"
)

const defaultHostname = "github.com"

// Remotes represents a set of git remotes
type Remotes []*Remote

// FindByName returns the first Remote whose name matches the list
func (r Remotes) FindByName(names ...string) (*Remote, error) {
	for _, name := range names {
		for _, rem := range r {
			if rem.Name == name || name == "*" {
				return rem, nil
			}
		}
	}
	return nil, fmt.Errorf("no GitHub remotes found")
}

// Remote represents a git remote mapped to a GitHub repository
type Remote struct {
	Name  string
	Owner string
	Repo  string
}

func (r *Remote) String() string {
	return r.Name
}

// GitHubRepository represents a GitHub respository
type GitHubRepository struct {
	Name  string
	Owner string
}

func parseRemotes() (remotes Remotes, err error) {
	re := regexp.MustCompile(`(.+)\s+(.+)\s+\((push|fetch)\)`)

	gitRemotes, err := git.Remotes()
	if err != nil {
		return
	}

	remotesMap := make(map[string]map[string]string)
	for _, r := range gitRemotes {
		if re.MatchString(r) {
			match := re.FindStringSubmatch(r)
			name := strings.TrimSpace(match[1])
			url := strings.TrimSpace(match[2])
			urlType := strings.TrimSpace(match[3])
			utm, ok := remotesMap[name]
			if !ok {
				utm = make(map[string]string)
				remotesMap[name] = utm
			}
			utm[urlType] = url
		}
	}

	for name, urlMap := range remotesMap {
		repo, err := repoFromURL(urlMap["fetch"])
		if err != nil {
			repo, err = repoFromURL(urlMap["push"])
		}
		if err == nil {
			remotes = append(remotes, &Remote{
				Name:  name,
				Owner: repo.Owner,
				Repo:  repo.Name,
			})
		}
	}

	return
}

func repoFromURL(u string) (*GitHubRepository, error) {
	url, err := git.ParseURL(u)
	if err != nil {
		return nil, err
	}
	if url.Hostname() != defaultHostname {
		return nil, fmt.Errorf("invalid hostname: %s", url.Hostname())
	}
	parts := strings.SplitN(strings.TrimPrefix(url.Path, "/"), "/", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid path: %s", url.Path)
	}
	return &GitHubRepository{
		Owner: parts[0],
		Name:  strings.TrimSuffix(parts[1], ".git"),
	}, nil
}
