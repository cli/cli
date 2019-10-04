package github

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/utils"
)

type Project struct {
	Name     string
	Owner    string
	Host     string
	Protocol string
}

func (p Project) String() string {
	return fmt.Sprintf("%s/%s", p.Owner, p.Name)
}

func (p *Project) SameAs(other *Project) bool {
	return strings.ToLower(p.Owner) == strings.ToLower(other.Owner) &&
		strings.ToLower(p.Name) == strings.ToLower(other.Name) &&
		strings.ToLower(p.Host) == strings.ToLower(other.Host)
}

func (p *Project) WebURL(name, owner, path string) string {
	if owner == "" {
		owner = p.Owner
	}
	if name == "" {
		name = p.Name
	}

	ownerWithName := fmt.Sprintf("%s/%s", owner, name)
	if strings.Contains(ownerWithName, ".wiki") {
		ownerWithName = strings.TrimSuffix(ownerWithName, ".wiki")
		if path != "wiki" {
			if strings.HasPrefix(path, "commits") {
				path = "_history"
			} else if path != "" {
				path = fmt.Sprintf("_%s", path)
			}

			if path != "" {
				path = utils.ConcatPaths("wiki", path)
			} else {
				path = "wiki"
			}
		}
	}

	url := fmt.Sprintf("%s://%s", p.Protocol, utils.ConcatPaths(p.Host, ownerWithName))
	if path != "" {
		url = utils.ConcatPaths(url, path)
	}

	return url
}

func (p *Project) GitURL(name, owner string, isSSH bool) (url string) {
	if name == "" {
		name = p.Name
	}
	if owner == "" {
		owner = p.Owner
	}

	host := rawHost(p.Host)

	if preferredProtocol() == "https" {
		url = fmt.Sprintf("https://%s/%s/%s.git", host, owner, name)
	} else if isSSH || preferredProtocol() == "ssh" {
		url = fmt.Sprintf("git@%s:%s/%s.git", host, owner, name)
	} else {
		url = fmt.Sprintf("git://%s/%s/%s.git", host, owner, name)
	}

	return url
}

// Remove the scheme from host when the host url is absolute.
func rawHost(host string) string {
	u, err := url.Parse(host)
	utils.Check(err)

	if u.IsAbs() {
		return u.Host
	} else {
		return u.Path
	}
}

func preferredProtocol() string {
	userProtocol := os.Getenv("HUB_PROTOCOL")
	if userProtocol == "" {
		userProtocol, _ = git.Config("hub.protocol")
	}
	return userProtocol
}

func NewProjectFromURL(url *url.URL) (p *Project, err error) {
	if !knownGitHubHostsInclude(url.Host) {
		err = &GithubHostError{url}
		return
	}

	parts := strings.SplitN(url.Path, "/", 4)
	if len(parts) <= 2 {
		err = fmt.Errorf("Invalid GitHub URL: %s", url)
		return
	}

	name := strings.TrimSuffix(parts[2], ".git")
	p = newProject(parts[1], name, url.Host, url.Scheme)

	return
}

func NewProject(owner, name, host string) *Project {
	return newProject(owner, name, host, "")
}

func newProject(owner, name, host, protocol string) *Project {
	if strings.Contains(owner, "/") {
		result := strings.SplitN(owner, "/", 2)
		owner = result[0]
		if name == "" {
			name = result[1]
		}
	} else if strings.Contains(name, "/") {
		result := strings.SplitN(name, "/", 2)
		if owner == "" {
			owner = result[0]
		}
		name = result[1]
	}

	if host == "" {
		host = DefaultGitHubHost()
	}
	if host == "ssh.github.com" {
		host = GitHubHost
	}

	if protocol != "http" && protocol != "https" {
		protocol = ""
	}
	if protocol == "" {
		h := CurrentConfig().Find(host)
		if h != nil {
			protocol = h.Protocol
		}
	}
	if protocol == "" {
		protocol = "https"
	}

	if owner == "" {
		h := CurrentConfig().Find(host)
		if h != nil {
			owner = h.User
		}
	}

	return &Project{
		Name:     name,
		Owner:    owner,
		Host:     host,
		Protocol: protocol,
	}
}

func SanitizeProjectName(name string) string {
	name = filepath.Base(name)
	return strings.Replace(name, " ", "-", -1)
}
