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
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/internal/ghinstance"
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
	OpenInBrowser    func(string, string) error
	WriteSuccessHTML func(io.Writer)
	VerboseStream    io.Writer
	HTTPClient       *http.Client
	TimeNow          func() time.Time
	TimeSleep        func(time.Duration)
}

func detectDeviceFlow(statusCode int, values url.Values) (bool, error) {
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden ||
		statusCode == http.StatusNotFound || statusCode == http.StatusUnprocessableEntity ||
		(statusCode == http.StatusOK && values == nil) ||
		(statusCode == http.StatusBadRequest && values != nil && values.Get("error") == "unauthorized_client") {
		return true, nil
	} else if statusCode != http.StatusOK {
		if values != nil && values.Get("error_description") != "" {
			return false, fmt.Errorf("HTTP %d: %s", statusCode, values.Get("error_description"))
		}
		return false, fmt.Errorf("error: HTTP %d", statusCode)
	}
	return false, nil
}

// ObtainAccessToken guides the user through the browser OAuth flow on GitHub
// and returns the OAuth access token upon completion.
func (oa *OAuthFlow) ObtainAccessToken() (accessToken string, err error) {
	// first, check if OAuth Device Flow is supported
	initURL := fmt.Sprintf("https://%s/login/device/code", oa.Hostname)
	tokenURL := fmt.Sprintf("https://%s/login/oauth/access_token", oa.Hostname)

	oa.logf("POST %s\n", initURL)
	resp, err := oa.HTTPClient.PostForm(initURL, url.Values{
		"client_id": {oa.ClientID},
		"scope":     {strings.Join(oa.Scopes, " ")},
	})
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var values url.Values
	if strings.Contains(resp.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		var bb []byte
		bb, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		values, err = url.ParseQuery(string(bb))
		if err != nil {
			return
		}
	}

	if doFallback, err := detectDeviceFlow(resp.StatusCode, values); doFallback {
		// OAuth Device Flow is not available; continue with OAuth browser flow with a
		// local server endpoint as callback target
		return oa.localServerFlow()
	} else if err != nil {
		return "", fmt.Errorf("%v (%s)", err, initURL)
	}

	timeNow := oa.TimeNow
	if timeNow == nil {
		timeNow = time.Now
	}
	timeSleep := oa.TimeSleep
	if timeSleep == nil {
		timeSleep = time.Sleep
	}

	intervalSeconds, err := strconv.Atoi(values.Get("interval"))
	if err != nil {
		return "", fmt.Errorf("could not parse interval=%q as integer: %w", values.Get("interval"), err)
	}
	checkInterval := time.Duration(intervalSeconds) * time.Second

	expiresIn, err := strconv.Atoi(values.Get("expires_in"))
	if err != nil {
		return "", fmt.Errorf("could not parse expires_in=%q as integer: %w", values.Get("expires_in"), err)
	}
	expiresAt := timeNow().Add(time.Duration(expiresIn) * time.Second)

	err = oa.OpenInBrowser(values.Get("verification_uri"), values.Get("user_code"))
	if err != nil {
		return
	}

	for {
		timeSleep(checkInterval)
		accessToken, err = oa.deviceFlowPing(tokenURL, values.Get("device_code"))
		if accessToken == "" && err == nil {
			if timeNow().After(expiresAt) {
				err = errors.New("authentication timed out")
			} else {
				continue
			}
		}
		break
	}

	return
}

func (oa *OAuthFlow) deviceFlowPing(tokenURL, deviceCode string) (accessToken string, err error) {
	oa.logf("POST %s\n", tokenURL)
	resp, err := oa.HTTPClient.PostForm(tokenURL, url.Values{
		"client_id":   {oa.ClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error: HTTP %d (%s)", resp.StatusCode, tokenURL)
	}

	bb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	values, err := url.ParseQuery(string(bb))
	if err != nil {
		return "", err
	}

	if accessToken := values.Get("access_token"); accessToken != "" {
		return accessToken, nil
	}

	errorType := values.Get("error")
	if errorType == "authorization_pending" {
		return "", nil
	}

	if errorDescription := values.Get("error_description"); errorDescription != "" {
		return "", errors.New(errorDescription)
	}
	return "", errors.New("OAuth device flow error")
}

func (oa *OAuthFlow) localServerFlow() (accessToken string, err error) {
	state, _ := randomString(20)

	code := ""
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return
	}
	port := listener.Addr().(*net.TCPAddr).Port

	scopes := "repo"
	if oa.Scopes != nil {
		scopes = strings.Join(oa.Scopes, " ")
	}

	localhost := "127.0.0.1"
	callbackPath := "/callback"
	if ghinstance.IsEnterprise(oa.Hostname) {
		// the OAuth app on Enterprise hosts is still registered with a legacy callback URL
		// see https://github.com/cli/cli/pull/222, https://github.com/cli/cli/pull/650
		localhost = "localhost"
		callbackPath = "/"
	}

	q := url.Values{}
	q.Set("client_id", oa.ClientID)
	q.Set("redirect_uri", fmt.Sprintf("http://%s:%d%s", localhost, port, callbackPath))
	q.Set("scope", scopes)
	q.Set("state", state)

	startURL := fmt.Sprintf("https://%s/login/oauth/authorize?%s", oa.Hostname, q.Encode())
	oa.logf("open %s\n", startURL)
	err = oa.OpenInBrowser(startURL, "")
	if err != nil {
		return
	}

	_ = http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oa.logf("server handler: %s\n", r.URL.Path)
		if r.URL.Path != callbackPath {
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
	resp, err := oa.HTTPClient.PostForm(tokenURL,
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

	if resp.StatusCode != http.StatusOK {
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
