package auth

import (
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func randomString(length int) (string, error) {
	b := make([]byte, length/2)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// OAuthFlow represents the setup for authenticating with GitHub
type OAuthFlow struct {
	Hostname         string
	ClientID         string
	ClientSecret     string
	WriteSuccessHTML func(io.Writer)
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

	q := url.Values{}
	q.Set("client_id", oa.ClientID)
	q.Set("redirect_uri", fmt.Sprintf("http://localhost:%d", port))
	q.Set("scope", "repo")
	q.Set("state", state)

	startURL := fmt.Sprintf("https://%s/login/oauth/authorize?%s", oa.Hostname, q.Encode())
	if err := openInBrowser(startURL); err != nil {
		fmt.Fprintf(os.Stderr, "error opening web browser: %s\n", err)
		fmt.Fprintf(os.Stderr, "Please open the following URL manually:\n%s\n", startURL)
	}

	http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer listener.Close()
		rq := r.URL.Query()
		if state != rq.Get("state") {
			fmt.Fprintf(w, "Error: state mismatch")
			return
		}
		code = rq.Get("code")
		w.Header().Add("content-type", "text/html")
		if oa.WriteSuccessHTML != nil {
			oa.WriteSuccessHTML(w)
		} else {
			fmt.Fprintf(w, "<p>You have successfully authenticated. You may now close this page.</p>")
		}
	}))

	resp, err := http.PostForm(fmt.Sprintf("https://%s/login/oauth/access_token", oa.Hostname),
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
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	tokenValues, err := url.ParseQuery(string(body))
	if err != nil {
		return
	}
	accessToken = tokenValues.Get("access_token")
	return
}

func openInBrowser(url string) error {
	var args []string
	switch runtime.GOOS {
	case "darwin":
		args = []string{"open"}
	case "windows":
		args = []string{"cmd", "/c", "start"}
		r := strings.NewReplacer("&", "^&")
		url = r.Replace(url)
	default:
		args = []string{"xdg-open"}
	}

	args = append(args, url)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
