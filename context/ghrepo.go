package context

import (
	"fmt"
	"net/url"
	"os"
	"strings"
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
		repoURL, err = url.Parse(fmt.Sprintf("https://%s/%s.git", GitHubHostname(), repoFromEnv))
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
	if repoURL.Host != GitHubHostname() && repoURL.Host != fmt.Sprintf("ssh.%s", GitHubHostname()) {
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
		host = GitHubHostname()
	}
	if host == "ssh.github.com" {
		host = GitHubHostname()
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

	return &GitHubRepository{
		Name:     name,
		Owner:    owner,
		Host:     host,
		Protocol: protocol,
	}, nil

}
