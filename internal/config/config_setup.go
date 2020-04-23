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

// IsGitHubApp reports whether an OAuth app is "GitHub CLI" or "GitHub CLI (dev)"
func IsGitHubApp(id string) bool {
	// this intentionally doesn't use `oauthClientID` because that is a variable
	// that can potentially be changed at build time via GH_OAUTH_CLIENT_ID
	return id == "178c6fc778ccc68e1d6a" || id == "4d747ba5675d5d66553f"
}

func AuthFlow(notice string) (string, string, error) {
	var verboseStream io.Writer
	if strings.Contains(os.Getenv("DEBUG"), "oauth") {
		verboseStream = os.Stderr
	}

	flow := &auth.OAuthFlow{
		Hostname:     oauthHost,
		ClientID:     oauthClientID,
		ClientSecret: oauthClientSecret,
		Scopes:       []string{"repo", "read:org"},
		WriteSuccessHTML: func(w io.Writer) {
			fmt.Fprintln(w, oauthSuccessPage)
		},
		VerboseStream: verboseStream,
	}

	fmt.Fprintln(os.Stderr, notice)
	fmt.Fprintf(os.Stderr, "Press Enter to open %s in your browser... ", flow.Hostname)
	_ = waitForEnter(os.Stdin)
	token, err := flow.ObtainAccessToken()
	if err != nil {
		return "", "", err
	}

	userLogin, err := getViewer(token)
	if err != nil {
		return "", "", err
	}

	return token, userLogin, nil
}

func AuthFlowComplete() {
	fmt.Fprintln(os.Stderr, "Authentication complete. Press Enter to continue... ")
	_ = waitForEnter(os.Stdin)
}

// FIXME: make testable
func setupConfigFile(filename string) (Config, error) {
	token, userLogin, err := AuthFlow("Notice: authentication required")
	if err != nil {
		return nil, err
	}

	// TODO this sucks. It precludes us laying out a nice config with comments and such.
	type yamlConfig struct {
		Hosts map[string]map[string]string
	}

	yamlHosts := map[string]map[string]string{}
	yamlHosts[oauthHost] = map[string]string{}
	yamlHosts[oauthHost]["user"] = userLogin
	yamlHosts[oauthHost]["oauth_token"] = token

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
	_, err = cfgFile.Write(yamlData)
	if err != nil {
		return nil, err
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
