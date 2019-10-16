package context

import (
	"strings"

	"github.com/github/gh-cli/git"
	"github.com/mitchellh/go-homedir"
)

// Context represents the interface for querying information about the current environment
type Context interface {
	AuthToken() (string, error)
	AuthLogin() (string, error)
	Branch() (string, error)
	SetBranch(string)
	Remotes() (Remotes, error)
	BaseRepo() (*GitHubRepository, error)
	SetBaseRepo(string)
}

var currentContext Context

// Current returns the currently initialized Context instance
func Current() Context {
	return currentContext
}

// InitDefaultContext initializes the default filesystem context
func InitDefaultContext() Context {
	ctx := &fsContext{}
	if currentContext == nil {
		currentContext = ctx
	}
	return ctx
}

// A Context implementation that queries the filesystem
type fsContext struct {
	config   *configEntry
	remotes  Remotes
	branch   string
	baseRepo *GitHubRepository
}

func (c *fsContext) configFile() string {
	dir, _ := homedir.Expand("~/.config/hub")
	return dir
}

func (c *fsContext) getConfig() (*configEntry, error) {
	if c.config == nil {
		entry, err := parseConfigFile(c.configFile())
		if err != nil {
			return nil, err
		}
		c.config = entry
	}
	return c.config, nil
}

func (c *fsContext) AuthToken() (string, error) {
	config, err := c.getConfig()
	if err != nil {
		return "", err
	}
	return config.Token, nil
}

func (c *fsContext) AuthLogin() (string, error) {
	config, err := c.getConfig()
	if err != nil {
		return "", err
	}
	return config.User, nil
}

func (c *fsContext) Branch() (string, error) {
	if c.branch != "" {
		return c.branch, nil
	}

	currentBranch, err := git.Head()
	if err != nil {
		return "", err
	}

	c.branch = strings.Replace(currentBranch, "refs/heads/", "", 1)
	return c.branch, nil
}

func (c *fsContext) SetBranch(b string) {
	c.branch = b
}

func (c *fsContext) Remotes() (Remotes, error) {
	if c.remotes == nil {
		rem, err := parseRemotes()
		if err != nil {
			return nil, err
		}
		c.remotes = rem
	}
	return c.remotes, nil
}

func (c *fsContext) BaseRepo() (*GitHubRepository, error) {
	if c.baseRepo != nil {
		return c.baseRepo, nil
	}

	remotes, err := c.Remotes()
	if err != nil {
		return nil, err
	}
	rem, err := remotes.FindByName("upstream", "github", "origin", "*")
	if err != nil {
		return nil, err
	}

	c.baseRepo = &GitHubRepository{
		Owner: rem.Owner,
		Name:  rem.Repo,
	}
	return c.baseRepo, nil
}

func (c *fsContext) SetBaseRepo(nwo string) {
	parts := strings.SplitN(nwo, "/", 2)
	if len(parts) == 2 {
		c.baseRepo = &GitHubRepository{
			Owner: parts[0],
			Name:  parts[1],
		}
	}
}
