package context

import (
	"fmt"
	"strings"
)

// InitBlankContext initializes a blank context for testing
func InitBlankContext() Context {
	currentContext = &blankContext{
		authToken: "OTOKEN",
		authLogin: "monalisa",
	}
	return currentContext
}

// A Context implementation that queries the filesystem
type blankContext struct {
	authToken string
	authLogin string
	branch    string
	baseRepo  *GitHubRepository
}

func (c *blankContext) AuthToken() (string, error) {
	return c.authToken, nil
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

func (c *blankContext) BaseRepo() (*GitHubRepository, error) {
	if c.baseRepo == nil {
		return nil, fmt.Errorf("base repo was not initialized")
	}
	return c.baseRepo, nil
}

func (c *blankContext) SetBaseRepo(nwo string) {
	parts := strings.SplitN(nwo, "/", 2)
	if len(parts) == 2 {
		c.baseRepo = &GitHubRepository{
			Owner: parts[0],
			Name:  parts[1],
		}
	}
}
