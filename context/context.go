package context

import (
	"fmt"
	"io/ioutil"
	"os/user"
	"regexp"
	"strings"

	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/github"
)

type Context struct {
	Token    string
	Username string
	Branch   string
	GHRepo   *GitHubRepository
}

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

func GetContext() (*Context, error) {
	errors := []error{}
	token, terr := GetToken()
	if terr != nil {
		errors = append(errors, terr)
	}

	username, uerr := CurrentUsername()
	if uerr != nil {
		errors = append(errors, uerr)
	}

	branch, berr := CurrentBranch()
	if berr != nil {
		errors = append(errors, berr)
	}

	ghrepo, ghrerr := CurrentGitHubRepository()
	if ghrerr != nil {
		errors = append(errors, ghrerr)
	}

	var err error = nil

	if len(errors) > 0 {
		errStrings := []string{}
		for _, e := range errors {
			errStrings = append(errStrings, e.Error())
		}

		err = fmt.Errorf(strings.Join(errStrings, ", "))
	}

	return &Context{
		Token:    token,
		Username: username,
		Branch:   branch,
		GHRepo:   ghrepo,
	}, err
}
