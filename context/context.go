package context

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/github"
	"github.com/github/gh-cli/utils"
)

// GitRepository represents a git repo on local disk.
type GitRepository struct {
	// hmmm
}

func (GitRepository) github() (github.Repository, error) {
	return github.Repository{}, nil
}

func CurrentBranch() (string, error) {
	currentBranch, err := git.Head()
	if err != nil {
		return "", err
	}

	return strings.Replace(currentBranch, "refs/heads/", "", 1), nil
}

// CurrentGitRepository returns a representation of the current (ie, cwd) git
// repo on disk. It acts as a gateway to github resources tied to this local
// repo.
func CurrentGitRepository() (GitRepository, error) {
	return GitRepository{}, nil
}

// TODO continue to determine feasibility of porting Project over
type GitHubRepository struct {
	Name     string
	Owner    string
	Host     string
	Protocol string
}

func (ghRepo *GitHubRepository) WebURL(name, owner, path string) string {
	if owner == "" {
		owner = ghRepo.Owner
	}
	if name == "" {
		name = ghRepo.Name
	}

	ownerWithName := fmt.Sprintf("%s/%s", owner, name)
	url := fmt.Sprintf("%s://%s", ghRepo.Protocol, utils.ConcatPaths(ghRepo.Host, ownerWithName))
	if path != "" {
		url = utils.ConcatPaths(url, path)
	}

	return url
}

func (ghRepo *GitHubRepository) Client() *github.Client {
	return github.NewClient(ghRepo.Host)
}

func (ghRepo *GitHubRepository) GetPullRequests(filterParams map[string]interface{}, limit int, filter func(*github.PullRequest) bool) (pulls []github.PullRequest, err error) {
	client := ghRepo.Client()

	return client.FetchPullRequests(ghRepo.Owner, ghRepo.Name, filterParams, limit, filter)

}

func (ghRepo *GitHubRepository) GetPullRequestByCurrentBranch() (*github.PullRequest, error) {

	currentBranch, err := CurrentBranch()
	if err != nil {
		return nil, err
	}

	headWithOwner := fmt.Sprintf("%s:%s", ghRepo.Owner, currentBranch)

	filterParams := map[string]interface{}{"head": headWithOwner}

	prs, prerr := ghRepo.GetPullRequests(filterParams, 10, nil)
	if prerr != nil {
		return nil, err
	}
	if len(prs) == 0 {
		return nil, fmt.Errorf("no pull requests found for the current branch")
	}

	return &prs[0], nil
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
		remote, rerr := github.GuessRemote()

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

// GetPullRequestByNumber uses the GitHub API to fetch a pull request by its numerical ID.
func (GitRepository) GetPullRequestByNumber(number int) (*github.PullRequest, error) {
	// TODO once CurrentGitRepository works and can replace project() in pr.go
	return nil, nil

}
