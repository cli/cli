package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/cli/cli/pkg/browser"
)

func randomString(length int) (string, error) {
	b := make([]byte, length/2)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// OAuthFlow represents the setup for authenticating with GitHub
type OAuthFlow struct {
	Hostname         string
	ClientID         string
	ClientSecret     string
	Scopes           []string
	WriteSuccessHTML func(io.Writer)
	VerboseStream    io.Writer
}

// ObtainAccessToken guides the user through the browser OAuth flow on GitHub
// and returns the OAuth access token upon completion.
func (oa *OAuthFlow) ObtainAccessToken() (accessToken string, err error) {
	state, _ := randomString(20)

	code := ""
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return
	}
	port := listener.Addr().(*net.TCPAddr).Port

	scopes := "repo"
	if oa.Scopes != nil {
		scopes = strings.Join(oa.Scopes, " ")
	}

	q := url.Values{}
	q.Set("client_id", oa.ClientID)
	q.Set("redirect_uri", fmt.Sprintf("http://127.0.0.1:%d/callback", port))
	q.Set("scope", scopes)
	q.Set("state", state)

	startURL := fmt.Sprintf("https://%s/login/oauth/authorize?%s", oa.Hostname, q.Encode())
	oa.logf("open %s\n", startURL)
	if err := openInBrowser(startURL); err != nil {
		fmt.Fprintf(os.Stderr, "error opening web browser: %s\n", err)
		fmt.Fprintf(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "Please open the following URL manually:\n%s\n", startURL)
		fmt.Fprintf(os.Stderr, "")
		// TODO: Temporary workaround for https://github.com/cli/cli/issues/297
		fmt.Fprintf(os.Stderr, "If you are on a server or other headless system, use this workaround instead:")
		fmt.Fprintf(os.Stderr, "  1. Complete authentication on a GUI system")
		fmt.Fprintf(os.Stderr, "  2. Copy the contents of ~/.config/gh/config.yml to this system")
	}

	_ = http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oa.logf("server handler: %s\n", r.URL.Path)
		if r.URL.Path != "/callback" {
			w.WriteHeader(404)
			return
		}
		defer listener.Close()
		rq := r.URL.Query()
		if state != rq.Get("state") {
			fmt.Fprintf(w, "Error: state mismatch")
			return
		}
		code = rq.Get("code")
		oa.logf("server received code %q\n", code)
		w.Header().Add("content-type", "text/html")
		if oa.WriteSuccessHTML != nil {
			oa.WriteSuccessHTML(w)
		} else {
			fmt.Fprintf(w, "<p>You have successfully authenticated. You may now close this page.</p>")
		}
	}))

	tokenURL := fmt.Sprintf("https://%s/login/oauth/access_token", oa.Hostname)
	oa.logf("POST %s\n", tokenURL)
	resp, err := http.PostForm(tokenURL,
		url.Values{
			"client_id":     {oa.ClientID},
			"client_secret": {oa.ClientSecret},
			"code":          {code},
			"state":         {state},
		})
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err = fmt.Errorf("HTTP %d error while obtaining OAuth access token", resp.StatusCode)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	tokenValues, err := url.ParseQuery(string(body))
	if err != nil {
		return
	}
	accessToken = tokenValues.Get("access_token")
	if accessToken == "" {
		err = errors.New("the access token could not be read from HTTP response")
	}
	return
}

func (oa *OAuthFlow) logf(format string, args ...interface{}) {
	if oa.VerboseStream == nil {
		return
	}
	fmt.Fprintf(oa.VerboseStream, format, args...)
}

func openInBrowser(url string) error {
	cmd, err := browser.Command(url)
	if err != nil {
		return err
	}
	return cmd.Run()
}
