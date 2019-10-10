package context

import (
	"io/ioutil"
	"os/user"
	"regexp"
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
