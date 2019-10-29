package context

import (
	"fmt"
	"strings"
)

// NewBlank initializes a blank Context suitable for testing
func NewBlank() Context {
	return &blankContext{}
}

// A Context implementation that queries the filesystem
type blankContext struct {
	authToken string
	authLogin string
	branch    string
	baseRepo  GitHubRepository
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

func (c *blankContext) AuthToken() (string, error) {
	return c.authToken, nil
}

func (c *blankContext) SetAuthToken(t string) {
	c.authToken = t
}

func (c *blankContext) AuthLogin() (string, error) {
	return c.authLogin, nil
}

func (c *blankContext) Branch() (string, error) {
	if c.branch == "" {
		return "", fmt.Errorf("branch was not initialized")
	}
	return c.branch, nil
}

func (c *blankContext) SetBranch(b string) {
	c.branch = b
}

func (c *blankContext) Remotes() (Remotes, error) {
	return Remotes{}, nil
}

func (c *blankContext) BaseRepo() (GitHubRepository, error) {
	if c.baseRepo == nil {
		return nil, fmt.Errorf("base repo was not initialized")
	}
	return c.baseRepo, nil
}

func (c *blankContext) SetBaseRepo(nwo string) {
	parts := strings.SplitN(nwo, "/", 2)
	if len(parts) == 2 {
		c.baseRepo = &ghRepo{parts[0], parts[1]}
	}
}
