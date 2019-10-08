package context

import (
	"strings"

	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/github"
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

func CurrentRepository() (GitRepository, error) {
	return GitRepository{}, nil
}
