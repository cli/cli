package authflow

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/oauth"
)

var (
	// The "GitHub CLI" OAuth app
	oauthClientID = "178c6fc778ccc68e1d6a"
	// This value is safe to be embedded in version control
	oauthClientSecret = "34ddeff2b558a23d38fba8a6de74f086ede1cc0b"
)

type iconfig interface {
	Set(string, string, string) error
	Write() error
}

func AuthFlowWithConfig(cfg iconfig, IO *iostreams.IOStreams, hostname, notice string, additionalScopes []string) (string, error) {
	// TODO this probably shouldn't live in this package. It should probably be in a new package that
	// depends on both iostreams and config.
	stderr := IO.ErrOut
	cs := IO.ColorScheme()

	token, userLogin, err := authFlow(hostname, IO, notice, additionalScopes)
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

	fmt.Fprintf(stderr, "%s Authentication complete. %s to continue...\n",
		cs.SuccessIcon(), cs.Bold("Press Enter"))
	_ = waitForEnter(IO.In)

	return token, nil
}

func authFlow(oauthHost string, IO *iostreams.IOStreams, notice string, additionalScopes []string) (string, string, error) {
	w := IO.ErrOut
	cs := IO.ColorScheme()

	httpClient := http.DefaultClient
	if envDebug := os.Getenv("DEBUG"); envDebug != "" {
		logTraffic := strings.Contains(envDebug, "api") || strings.Contains(envDebug, "oauth")
		httpClient.Transport = api.VerboseLog(IO.ErrOut, logTraffic, IO.ColorEnabled())(httpClient.Transport)
	}

	minimumScopes := []string{"repo", "read:org", "gist"}
	scopes := append(minimumScopes, additionalScopes...)

	callbackURI := "http://127.0.0.1/callback"
	if ghinstance.IsEnterprise(oauthHost) {
		// the OAuth app on Enterprise hosts is still registered with a legacy callback URL
		// see https://github.com/cli/cli/pull/222, https://github.com/cli/cli/pull/650
		callbackURI = "http://localhost/"
	}

	flow := &oauth.Flow{
		Hostname:     oauthHost,
		ClientID:     oauthClientID,
		ClientSecret: oauthClientSecret,
		CallbackURI:  callbackURI,
		Scopes:       scopes,
		DisplayCode: func(code, verificationURL string) error {
			fmt.Fprintf(w, "%s First copy your one-time code: %s\n", cs.Yellow("!"), cs.Bold(code))
			return nil
		},
		BrowseURL: func(url string) error {
			fmt.Fprintf(w, "- %s to open %s in your browser... ", cs.Bold("Press Enter"), oauthHost)
			_ = waitForEnter(IO.In)

			// FIXME: read the browser from cmd Factory rather than recreating it
			browser := cmdutil.NewBrowser(os.Getenv("BROWSER"), IO.Out, IO.ErrOut)
			if err := browser.Browse(url); err != nil {
				fmt.Fprintf(w, "%s Failed opening a web browser at %s\n", cs.Red("!"), url)
				fmt.Fprintf(w, "  %s\n", err)
				fmt.Fprint(w, "  Please try entering the URL in your browser manually\n")
			}
			return nil
		},
		WriteSuccessHTML: func(w io.Writer) {
			fmt.Fprintln(w, oauthSuccessPage)
		},
		HTTPClient: httpClient,
		Stdin:      IO.In,
		Stdout:     w,
	}

	fmt.Fprintln(w, notice)

	token, err := flow.DetectFlow()
	if err != nil {
		return "", "", err
	}

	userLogin, err := getViewer(oauthHost, token.Token)
	if err != nil {
		return "", "", err
	}

	return token.Token, userLogin, nil
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
