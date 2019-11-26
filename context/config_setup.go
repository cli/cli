package context

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/auth"
	"gopkg.in/yaml.v3"
)

const (
	oauthHost = "github.com"
)

var (
	// The GitHub app that is meant for development
	oauthClientID = "4d747ba5675d5d66553f"
	// This value is safe to be embedded in version control
	oauthClientSecret = "d4fee7b3f9c2ef4284a5ca7be0ee200cf15b6f8d"
)

// TODO: have a conversation about whether this belongs in the "context" package
// FIXME: make testable
func setupConfigFile(filename string) (*configEntry, error) {
	flow := &auth.OAuthFlow{
		Hostname:     oauthHost,
		ClientID:     oauthClientID,
		ClientSecret: oauthClientSecret,
		WriteSuccessHTML: func(w io.Writer) {
			fmt.Fprintln(w, oauthSuccessPage)
		},
	}

	fmt.Fprintln(os.Stderr, "Notice: authentication required")
	fmt.Fprintf(os.Stderr, "Press Enter to open %s in your web browser... ", flow.Hostname)
	waitForEnter(os.Stdin)
	token, err := flow.ObtainAccessToken()
	if err != nil {
		return nil, err
	}

	userLogin, err := getViewer(token)
	if err != nil {
		return nil, err
	}
	entry := configEntry{
		User:  userLogin,
		Token: token,
	}
	data := make(map[string][]configEntry)
	data[flow.Hostname] = []configEntry{entry}

	err = os.MkdirAll(filepath.Dir(filename), 0771)
	if err != nil {
		return nil, err
	}

	config, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	defer config.Close()

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return nil, err
	}
	n, err := config.Write(yamlData)
	if err == nil && n < len(yamlData) {
		err = io.ErrShortWrite
	}

	if err == nil {
		fmt.Fprintln(os.Stderr, "Authentication complete. Press Enter to continue... ")
		waitForEnter(os.Stdin)
	}

	return &entry, err
}

func getViewer(token string) (string, error) {
	http := api.NewClient(api.AddHeader("Authorization", fmt.Sprintf("token %s", token)))

	response := struct {
		Viewer struct {
			Login string
		}
	}{}
	err := http.GraphQL("{ viewer { login } }", nil, &response)
	return response.Viewer.Login, err
}

func waitForEnter(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Scan()
	return scanner.Err()
}
