package context

import (
	"path"

	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/mitchellh/go-homedir"
)

// Context represents the interface for querying information about the current environment
type Context interface {
	AuthToken() (string, error)
	SetAuthToken(string)
	AuthLogin() (string, error)
	Branch() (string, error)
	SetBranch(string)
	Remotes() (Remotes, error)
	BaseRepo() (ghrepo.Interface, error)
	SetBaseRepo(string)
}

// New initializes a Context that reads from the filesystem
func New() Context {
	return &fsContext{}
}

// A Context implementation that queries the filesystem
type fsContext struct {
	config    *configEntry
	remotes   Remotes
	branch    string
	baseRepo  ghrepo.Interface
	authToken string
}

func ConfigDir() string {
	dir, _ := homedir.Expand("~/.config/gh")
	return dir
}

func configFile() string {
	return path.Join(ConfigDir(), "config.yml")
}

func (c *fsContext) getConfig() (*configEntry, error) {
	if c.config == nil {
		entry, err := parseOrSetupConfigFile(configFile())
		if err != nil {
			return nil, err
		}
		c.config = entry
		c.authToken = ""
	}
	return c.config, nil
}

func (c *fsContext) AuthToken() (string, error) {
	if c.authToken != "" {
		return c.authToken, nil
	}

	config, err := c.getConfig()
	if err != nil {
		return "", err
	}
	return config.Token, nil
}

func (c *fsContext) SetAuthToken(t string) {
	c.authToken = t
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

	currentBranch, err := git.CurrentBranch()
	if err != nil {
		return "", err
	}

	c.branch = currentBranch
	return c.branch, nil
}

func (c *fsContext) SetBranch(b string) {
	c.branch = b
}

func (c *fsContext) Remotes() (Remotes, error) {
	if c.remotes == nil {
		gitRemotes, err := git.Remotes()
		if err != nil {
			return nil, err
		}
		sshTranslate := git.ParseSSHConfig().Translator()
		c.remotes = translateRemotes(gitRemotes, sshTranslate)
	}
	return c.remotes, nil
}

func (c *fsContext) BaseRepo() (ghrepo.Interface, error) {
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

	c.baseRepo = rem
	return c.baseRepo, nil
}

func (c *fsContext) SetBaseRepo(nwo string) {
	c.baseRepo = ghrepo.FromFullName(nwo)
}
