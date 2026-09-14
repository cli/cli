package authflow

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/atotto/clipboard"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/oauth"

	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

var (
	// The "GitHub CLI" OAuth app
	oauthClientID = "178c6fc778ccc68e1d6a"
	// This value is safe to be embedded in version control
	oauthClientSecret = "34ddeff2b558a23d38fba8a6de74f086ede1cc0b"
)

// AuthResult carries the outcome of AuthFlow. Refreshable is set only when the server issued a refresh token, so
// callers can store a refreshable credential in that case and a plain token otherwise.
type AuthResult struct {
	Token       string
	Username    string
	Refreshable *gh.Credential
}

// AuthFlow initiates an OAuth device or web application flow to acquire a
// token. The provided HTTP client should be a plain client that does not set
// auth or other headers. When requestRefreshToken is true, gh asks the server
// for a short-lived, refreshable credential; the server may still issue a
// non-expiring token, in which case AuthResult.Refreshable is nil.
func AuthFlow(httpClient *http.Client, oauthHost string, IO *iostreams.IOStreams, notice string, additionalScopes []string, isInteractive bool, b browser.Browser, isCopyToClipboard bool, requestRefreshToken bool) (*AuthResult, error) {
	w := IO.ErrOut
	cs := IO.ColorScheme()

	minimumScopes := []string{"repo", "read:org", "gist"}
	scopes := append(minimumScopes, additionalScopes...)

	host, err := oauth.NewGitHubHost(ghinstance.HostPrefix(oauthHost))
	if err != nil {
		return nil, err
	}

	flow := &oauth.Flow{
		Host:                host,
		ClientID:            oauthClientID,
		ClientSecret:        oauthClientSecret,
		CallbackURI:         getCallbackURI(oauthHost),
		Scopes:              scopes,
		RequestRefreshToken: requestRefreshToken,
		DisplayCode: func(code, verificationURL string) error {
			if isCopyToClipboard {
				err := clipboard.WriteAll(code)
				if err == nil {
					fmt.Fprintf(w, "%s One-time code (%s) copied to clipboard\n", cs.Yellow("!"), cs.Bold(code))
					return nil
				}
				fmt.Fprintf(w, "%s Failed to copy one-time code to clipboard\n", cs.Red("!"))
				fmt.Fprintf(w, "  %s\n", err)
			}
			fmt.Fprintf(w, "%s First copy your one-time code: %s\n", cs.Yellow("!"), cs.Bold(code))
			return nil
		},
		BrowseURL: func(authURL string) error {
			if u, err := url.Parse(authURL); err == nil {
				if u.Scheme != "http" && u.Scheme != "https" {
					return fmt.Errorf("invalid URL: %s", authURL)
				}
			} else {
				return err
			}

			if !isInteractive {
				fmt.Fprintf(w, "%s to continue in your web browser: %s\n", cs.Bold("Open this URL"), authURL)
				return nil
			}

			fmt.Fprintf(w, "%s to open %s in your browser... ", cs.Bold("Press Enter"), authURL)
			_ = waitForEnter(IO.In)

			if err := b.Browse(authURL); err != nil {
				fmt.Fprintf(w, "%s Failed opening a web browser at %s\n", cs.Red("!"), authURL)
				fmt.Fprintf(w, "  %s\n", err)
				fmt.Fprint(w, "  Please try entering the URL in your browser manually\n")
			}
			return nil
		},
		WriteSuccessHTML: func(w io.Writer) {
			fmt.Fprint(w, oauthSuccessPage)
		},
		HTTPClient: httpClient,
		Stdin:      IO.In,
		Stdout:     w,
	}

	fmt.Fprintln(w, notice)

	// Anchor any short-lived token's lifetime to the instant before the flow begins. DetectFlow blocks through the
	// user's browser interaction and the server issues the token at the end, so this instant is at or before issuance
	// and never overestimates the expiry. Use UTC so the derived absolute expiry matches the canonical UTC form gh
	// stores and displays.
	requestedAt := timeNow().UTC()
	token, err := flow.DetectFlow()
	if err != nil {
		return nil, err
	}

	userLogin, err := getViewer(httpClient, oauthHost, token.Token)
	if err != nil {
		return nil, err
	}

	result := &AuthResult{
		Token:    token.Token,
		Username: userLogin,
	}
	// A refresh token is present only when the server honored the short-lived request, so gate the refreshable
	// credential on it rather than on requestRefreshToken.
	if token.RefreshToken != "" {
		credential := credentialFromAccessToken(token, requestedAt)
		result.Refreshable = &credential
	}

	return result, nil
}

func getCallbackURI(oauthHost string) string {
	callbackURI := "http://127.0.0.1/callback"
	if ghauth.IsEnterprise(oauthHost) {
		// the OAuth app on Enterprise hosts is still registered with a legacy callback URL
		// see https://github.com/cli/cli/pull/222, https://github.com/cli/cli/pull/650
		callbackURI = "http://localhost/"
	}
	return callbackURI
}

type cfg struct {
	token string
}

func (c cfg) ActiveTokenWithRefresh(hostname string) (gh.Credential, gh.RefreshStatus, error) {
	return gh.Credential{Token: c.token}, gh.RefreshStatusInapplicable, nil
}

// HostForAPIHost never resolves, because the token here is supplied directly by
// the login flow and is used for whatever host the request is aimed at.
func (c cfg) HostForAPIHost(string) (string, bool) {
	return "", false
}

// APIHostForHost never resolves for the same reason as HostForAPIHost.
func (c cfg) APIHostForHost(string) (string, bool) {
	return "", false
}

func getViewer(httpClient *http.Client, hostname, token string) (string, error) {
	authedClient := *httpClient
	authedClient.Transport = api.AddAuthTokenHeader(httpClient.Transport, cfg{token: token})
	return api.CurrentLoginName(api.NewClientFromHTTP(&authedClient), hostname)
}

func waitForEnter(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Scan()
	return scanner.Err()
}
