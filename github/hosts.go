package github

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/github/gh-cli/git"
)

var (
	GitHubHostEnv = os.Getenv("GITHUB_HOST")
	cachedHosts   []string
)

type GithubHostError struct {
	url *url.URL
}

func (e *GithubHostError) Error() string {
	return fmt.Sprintf("Invalid GitHub URL: %s", e.url)
}

func KnownGitHubHostsInclude(host string) bool {
	for _, hh := range knownGitHubHosts() {
		if hh == host {
			return true
		}
	}

	return false
}

func knownGitHubHosts() []string {
	if cachedHosts != nil {
		return cachedHosts
	}

	hosts := []string{}
	defaultHost := DefaultGitHubHost()
	hosts = append(hosts, defaultHost)
	hosts = append(hosts, "ssh.github.com")

	ghHosts, _ := git.ConfigAll("hub.host")
	for _, ghHost := range ghHosts {
		ghHost = strings.TrimSpace(ghHost)
		if ghHost != "" {
			hosts = append(hosts, ghHost)
		}
	}

	cachedHosts = hosts
	return hosts
}

func DefaultGitHubHost() string {
	defaultHost := GitHubHostEnv
	if defaultHost == "" {
		defaultHost = GitHubHost
	}

	return defaultHost
}
