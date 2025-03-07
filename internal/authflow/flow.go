package authflow

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/cli/oauth"
	"github.com/henvic/httpretty"

	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

var (
	// The "GitHub CLI" OAuth app
	oauthClientID = "178c6fc778ccc68e1d6a"
	// This value is safe to be embedded in version control
	oauthClientSecret = "34ddeff2b558a23d38fba8a6de74f086ede1cc0b"

	jsonTypeRE = regexp.MustCompile(`[/+]json($|;)`)
)

func AuthFlow(oauthHost string, IO *iostreams.IOStreams, notice string, additionalScopes []string, isInteractive bool, b browser.Browser) (string, string, error) {
	w := IO.ErrOut
	cs := IO.ColorScheme()

	httpClient := &http.Client{}
	debugEnabled, debugValue := utils.IsDebugEnabled()
	if debugEnabled {
		logTraffic := strings.Contains(debugValue, "api")
		httpClient.Transport = verboseLog(IO.ErrOut, logTraffic, IO.ColorEnabled())(httpClient.Transport)
	}

	minimumScopes := []string{"repo", "read:org", "gist"}
	scopes := append(minimumScopes, additionalScopes...)

	host, err := oauth.NewGitHubHost(ghinstance.HostPrefix(oauthHost))
	if err != nil {
		return "", "", err
	}

	flow := &oauth.Flow{
		Host:         host,
		ClientID:     oauthClientID,
		ClientSecret: oauthClientSecret,
		CallbackURI:  getCallbackURI(oauthHost),
		Scopes:       scopes,
		DisplayCode: func(code, verificationURL string) error {
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

	token, err := flow.DetectFlow()
	if err != nil {
		return "", "", err
	}

	userLogin, err := getViewer(oauthHost, token.Token, IO.ErrOut)
	if err != nil {
		return "", "", err
	}

	return token.Token, userLogin, nil
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

func (c cfg) ActiveToken(hostname string) (string, string) {
	return c.token, "oauth_token"
}

func getViewer(hostname, token string, logWriter io.Writer) (string, error) {
	opts := api.HTTPClientOptions{
		Config: cfg{token: token},
		Log:    logWriter,
	}
	client, err := api.NewHTTPClient(opts)
	if err != nil {
		return "", err
	}
	return api.CurrentLoginName(api.NewClientFromHTTP(client), hostname)
}

func waitForEnter(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Scan()
	return scanner.Err()
}

func verboseLog(out io.Writer, logTraffic bool, colorize bool) func(http.RoundTripper) http.RoundTripper {
	logger := &httpretty.Logger{
		Time:            true,
		TLS:             false,
		Colors:          colorize,
		RequestHeader:   logTraffic,
		RequestBody:     logTraffic,
		ResponseHeader:  logTraffic,
		ResponseBody:    logTraffic,
		Formatters:      []httpretty.Formatter{&httpretty.JSONFormatter{}},
		MaxResponseBody: 10000,
	}
	logger.SetOutput(out)
	logger.SetBodyFilter(func(h http.Header) (skip bool, err error) {
		return !inspectableMIMEType(h.Get("Content-Type")), nil
	})
	return logger.RoundTripper
}

func inspectableMIMEType(t string) bool {
	return strings.HasPrefix(t, "text/") ||
		strings.HasPrefix(t, "application/x-www-form-urlencoded") ||
		jsonTypeRE.MatchString(t)
}
