package config

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/auth"
	"github.com/cli/cli/pkg/browser"
	"github.com/cli/cli/utils"
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

func AuthFlowWithConfig(cfg Config, hostname, notice string, additionalScopes []string) (string, error) {
	token, userLogin, err := authFlow(hostname, notice, additionalScopes)
	if err != nil {
		return "", err
	}

	err = cfg.Set(hostname, "user", userLogin)
	if err != nil {
		return "", err
	}
	err = cfg.Set(hostname, "oauth_token", token)
	if err != nil {
		return "", err
	}

	err = cfg.Write()
	if err != nil {
		return "", err
	}

	AuthFlowComplete()
	return token, nil
}

func authFlow(oauthHost, notice string, additionalScopes []string) (string, string, error) {
	var verboseStream io.Writer
	if strings.Contains(os.Getenv("DEBUG"), "oauth") {
		verboseStream = os.Stderr
	}

	minimumScopes := []string{"repo", "read:org", "gist"}
	scopes := append(minimumScopes, additionalScopes...)

	flow := &auth.OAuthFlow{
		Hostname:     oauthHost,
		ClientID:     oauthClientID,
		ClientSecret: oauthClientSecret,
		Scopes:       scopes,
		WriteSuccessHTML: func(w io.Writer) {
			fmt.Fprintln(w, oauthSuccessPage)
		},
		VerboseStream: verboseStream,
		HTTPClient:    http.DefaultClient,
		OpenInBrowser: func(url, code string) error {
			if code != "" {
				fmt.Fprintf(os.Stderr, "%s First copy your one-time code: %s\n", utils.Yellow("!"), utils.Bold(code))
			}
			fmt.Fprintf(os.Stderr, "- %s to open %s in your browser... ", utils.Bold("Press Enter"), oauthHost)
			_ = waitForEnter(os.Stdin)

			browseCmd, err := browser.Command(url)
			if err != nil {
				return err
			}
			err = browseCmd.Run()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s Failed opening a web browser at %s\n", utils.Red("!"), url)
				fmt.Fprintf(os.Stderr, "  %s\n", err)
				fmt.Fprint(os.Stderr, "  Please try entering the URL in your browser manually\n")
			}
			return nil
		},
	}

	fmt.Fprintln(os.Stderr, notice)

	token, err := flow.ObtainAccessToken()
	if err != nil {
		return "", "", err
	}

	userLogin, err := getViewer(oauthHost, token)
	if err != nil {
		return "", "", err
	}

	return token, userLogin, nil
}

func AuthFlowComplete() {
	fmt.Fprintf(os.Stderr, "%s Authentication complete. %s to continue...\n",
		utils.GreenCheck(), utils.Bold("Press Enter"))
	_ = waitForEnter(os.Stdin)
}

func getViewer(hostname, token string) (string, error) {
	http := api.NewClient(api.AddHeader("Authorization", fmt.Sprintf("token %s", token)))
	return api.CurrentLoginName(http, hostname)
}

func waitForEnter(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Scan()
	return scanner.Err()
}
