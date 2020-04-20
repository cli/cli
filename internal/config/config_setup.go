package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/auth"
	"gopkg.in/yaml.v3"
)

const (
	oauthHost = "github.com"
)

var (
	// The "GitHub CLI" OAuth app
	oauthClientID = "178c6fc778ccc68e1d6a"
	// This value is safe to be embedded in version control
	oauthClientSecret = "34ddeff2b558a23d38fba8a6de74f086ede1cc0b"
)

// TODO: have a conversation about whether this belongs in the "context" package
// FIXME: make testable
func setupConfigFile(filename string) (Config, error) {
	var verboseStream io.Writer
	if strings.Contains(os.Getenv("DEBUG"), "oauth") {
		verboseStream = os.Stderr
	}

	flow := &auth.OAuthFlow{
		Hostname:     oauthHost,
		ClientID:     oauthClientID,
		ClientSecret: oauthClientSecret,
		WriteSuccessHTML: func(w io.Writer) {
			fmt.Fprintln(w, oauthSuccessPage)
		},
		VerboseStream: verboseStream,
	}

	fmt.Fprintln(os.Stderr, "Notice: authentication required")
	fmt.Fprintf(os.Stderr, "Press Enter to open %s in your browser... ", flow.Hostname)
	_ = waitForEnter(os.Stdin)
	token, err := flow.ObtainAccessToken()
	if err != nil {
		return nil, err
	}

	userLogin, err := getViewer(token)
	if err != nil {
		return nil, err
	}

	// TODO this sucks. It precludes us laying out a nice config with comments and such.
	type yamlConfig struct {
		Hosts map[string]map[string]string
	}

	yamlHosts := map[string]map[string]string{}
	yamlHosts[flow.Hostname] = map[string]string{}
	yamlHosts[flow.Hostname]["user"] = userLogin
	yamlHosts[flow.Hostname]["oauth_token"] = token

	defaultConfig := yamlConfig{
		Hosts: yamlHosts,
	}

	err = os.MkdirAll(filepath.Dir(filename), 0771)
	if err != nil {
		return nil, err
	}

	cfgFile, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	defer cfgFile.Close()

	yamlData, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return nil, err
	}
	n, err := cfgFile.Write(yamlData)
	if err == nil && n < len(yamlData) {
		err = io.ErrShortWrite
	}

	if err == nil {
		fmt.Fprintln(os.Stderr, "Authentication complete. Press Enter to continue... ")
		_ = waitForEnter(os.Stdin)
	}

	// TODO cleaner error handling? this "should" always work given that we /just/ wrote the file...
	return ParseConfig(filename)
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
