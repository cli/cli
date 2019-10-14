package context

import (
	"io/ioutil"
	"os/user"
	"regexp"
	"strings"

	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/github"
)

// GetToken returns the authentication token as stored in the config file for the user running gh-cli
func GetToken() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}

	content, err := ioutil.ReadFile(usr.HomeDir + "/.config/hub")
	if err != nil {
		return "", err
	}

	r := regexp.MustCompile(`oauth_token: (\w+)`)
	token := r.FindStringSubmatch(string(content))
	return token[1], nil
}

func CurrentUsername() (string, error) {
	host, err := github.CurrentConfig().DefaultHost()
	if err != nil {
		return "", nil
	}
	return host.User, nil
}

func CurrentBranch() (string, error) {
	currentBranch, err := git.Head()
	if err != nil {
		return "", err
	}

	return strings.Replace(currentBranch, "refs/heads/", "", 1), nil
}
