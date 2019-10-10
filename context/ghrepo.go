package context

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/github/gh-cli/github"
)

type GitHubRepository struct {
	Name     string
	Owner    string
	Host     string
	Protocol string
}

func CurrentGitHubRepository() (*GitHubRepository, error) {

	var repoURL *url.URL
	var err error
	if repoFromEnv := os.Getenv("GH_REPO"); repoFromEnv != "" {
		repoURL, err = url.Parse(fmt.Sprintf("https://github.com/%s.git", repoFromEnv))
		if err != nil {
			return nil, err
		}
	} else {
		remote, rerr := GuessRemote()

		if rerr != nil {
			return nil, rerr
		}
		repoURL = remote.URL
	}

	urlError := fmt.Errorf("invalid GitHub URL: %s", repoURL)
	if !github.KnownGitHubHostsInclude(repoURL.Host) {
		return nil, urlError
	}

	parts := strings.SplitN(repoURL.Path, "/", 4)
	if len(parts) <= 2 {
		return nil, urlError
	}

	name := strings.TrimSuffix(parts[2], ".git")
	owner := parts[1]
	host := repoURL.Host
	protocol := repoURL.Scheme

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
		host = github.DefaultGitHubHost()
	}
	if host == "ssh.github.com" {
		host = github.GitHubHost
	}

	if protocol != "http" && protocol != "https" {
		protocol = ""
	}
	if protocol == "" {
		h := github.CurrentConfig().Find(host)
		if h != nil {
			protocol = h.Protocol
		}
	}
	if protocol == "" {
		protocol = "https"
	}

	if owner == "" {
		h := github.CurrentConfig().Find(host)
		if h != nil {
			owner = h.User
		}
	}

	return &GitHubRepository{
		Name:     name,
		Owner:    owner,
		Host:     host,
		Protocol: protocol,
	}, nil

}
