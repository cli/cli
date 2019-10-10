package github

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/github/gh-cli/version"
)

const (
	GitHubHost  string = "github.com"
	OAuthAppURL string = "https://github.com/github/gh-cli"
)

var userAgent = "GitHub CLI " + version.Version

func NewClient(h string) *Client {
	return NewClientWithHost(&Host{Host: h})
}

func NewClientWithHost(host *Host) *Client {
	return &Client{Host: host}
}

type Client struct {
	Host         *Host
	cachedClient *simpleClient
}

type User struct {
	Login string `json:"login"`
}

func (client *Client) CurrentUser() (user *User, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.Get("user")
	if err = checkStatus(200, "getting current user", res, err); err != nil {
		return
	}

	user = &User{}
	err = res.Unmarshal(user)
	return
}

type AuthorizationEntry struct {
	Token string `json:"token"`
}

func isToken(api *simpleClient, password string) bool {
	api.PrepareRequest = func(req *http.Request) {
		req.Header.Set("Authorization", "token "+password)
	}

	res, _ := api.Get("user")
	if res != nil && res.StatusCode == 200 {
		return true
	}
	return false
}

func (client *Client) FindOrCreateToken(user, password, twoFactorCode string) (token string, err error) {
	api := client.apiClient()

	if len(password) >= 40 && isToken(api, password) {
		return password, nil
	}

	params := map[string]interface{}{
		"scopes":   []string{"repo"},
		"note_url": OAuthAppURL,
	}

	api.PrepareRequest = func(req *http.Request) {
		req.SetBasicAuth(user, password)
		if twoFactorCode != "" {
			req.Header.Set("X-GitHub-OTP", twoFactorCode)
		}
	}

	count := 1
	maxTries := 9
	for {
		params["note"], err = authTokenNote(count)
		if err != nil {
			return
		}

		res, postErr := api.PostJSON("authorizations", params)
		if postErr != nil {
			err = postErr
			break
		}

		if res.StatusCode == 201 {
			auth := &AuthorizationEntry{}
			if err = res.Unmarshal(auth); err != nil {
				return
			}
			token = auth.Token
			break
		} else if res.StatusCode == 422 && count < maxTries {
			count++
		} else {
			errInfo, e := res.ErrorInfo()
			if e == nil {
				err = errInfo
			} else {
				err = e
			}
			return
		}
	}

	return
}

func (client *Client) ensureAccessToken() error {
	if client.Host.AccessToken == "" {
		host, err := CurrentConfig().PromptForHost(client.Host.Host)
		if err != nil {
			return err
		}
		client.Host = host
	}
	return nil
}

func (client *Client) simpleApi() (c *simpleClient, err error) {
	err = client.ensureAccessToken()
	if err != nil {
		return
	}

	if client.cachedClient != nil {
		c = client.cachedClient
		return
	}

	c = client.apiClient()
	c.PrepareRequest = func(req *http.Request) {
		clientDomain := normalizeHost(client.Host.Host)
		if strings.HasPrefix(clientDomain, "api.github.") {
			clientDomain = strings.TrimPrefix(clientDomain, "api.")
		}
		requestHost := strings.ToLower(req.URL.Host)
		if requestHost == clientDomain || strings.HasSuffix(requestHost, "."+clientDomain) {
			req.Header.Set("Authorization", "token "+client.Host.AccessToken)
		}
	}

	client.cachedClient = c
	return
}

func (client *Client) apiClient() *simpleClient {
	unixSocket := os.ExpandEnv(client.Host.UnixSocket)
	httpClient := newHttpClient(os.Getenv("HUB_TEST_HOST"), os.Getenv("HUB_VERBOSE") != "", unixSocket)
	apiRoot := client.absolute(normalizeHost(client.Host.Host))
	if !strings.HasPrefix(apiRoot.Host, "api.github.") {
		apiRoot.Path = "/api/v3/"
	}

	return &simpleClient{
		httpClient: httpClient,
		rootUrl:    apiRoot,
	}
}

func (client *Client) absolute(host string) *url.URL {
	u, err := url.Parse("https://" + host + "/")
	if err != nil {
		panic(err)
	} else if client.Host != nil && client.Host.Protocol != "" {
		u.Scheme = client.Host.Protocol
	}
	return u
}

func normalizeHost(host string) string {
	if host == "" {
		return GitHubHost
	} else if strings.EqualFold(host, GitHubHost) {
		return "api.github.com"
	} else if strings.EqualFold(host, "github.localhost") {
		return "api.github.localhost"
	} else {
		return strings.ToLower(host)
	}
}

func checkStatus(expectedStatus int, action string, response *simpleResponse, err error) error {
	if err != nil {
		return fmt.Errorf("Error %s: %s", action, err.Error())
	} else if response.StatusCode != expectedStatus {
		errInfo, err := response.ErrorInfo()
		if err == nil {
			return FormatError(action, errInfo)
		} else {
			return fmt.Errorf("Error %s: %s (HTTP %d)", action, err.Error(), response.StatusCode)
		}
	} else {
		return nil
	}
}

func FormatError(action string, err error) (ee error) {
	switch e := err.(type) {
	default:
		ee = err
	case *errorInfo:
		statusCode := e.Response.StatusCode
		var reason string
		if s := strings.SplitN(e.Response.Status, " ", 2); len(s) >= 2 {
			reason = strings.TrimSpace(s[1])
		}

		errStr := fmt.Sprintf("Error %s: %s (HTTP %d)", action, reason, statusCode)

		var errorSentences []string
		for _, err := range e.Errors {
			switch err.Code {
			case "custom":
				errorSentences = append(errorSentences, err.Message)
			case "missing_field":
				errorSentences = append(errorSentences, fmt.Sprintf("Missing field: \"%s\"", err.Field))
			case "already_exists":
				errorSentences = append(errorSentences, fmt.Sprintf("Duplicate value for \"%s\"", err.Field))
			case "invalid":
				errorSentences = append(errorSentences, fmt.Sprintf("Invalid value for \"%s\"", err.Field))
			case "unauthorized":
				errorSentences = append(errorSentences, fmt.Sprintf("Not allowed to change field \"%s\"", err.Field))
			}
		}

		var errorMessage string
		if len(errorSentences) > 0 {
			errorMessage = strings.Join(errorSentences, "\n")
		} else {
			errorMessage = e.Message
			if action == "getting current user" && e.Message == "Resource not accessible by integration" {
				errorMessage = errorMessage + "\nYou must specify GITHUB_USER via environment variable."
			}
		}

		if errorMessage != "" {
			errStr = fmt.Sprintf("%s\n%s", errStr, errorMessage)
		}

		ee = fmt.Errorf(errStr)
	}

	return
}

func authTokenNote(num int) (string, error) {
	n := os.Getenv("USER")

	if n == "" {
		n = os.Getenv("USERNAME")
	}

	if n == "" {
		whoami := exec.Command("whoami")
		whoamiOut, err := whoami.Output()
		if err != nil {
			return "", err
		}
		n = strings.TrimSpace(string(whoamiOut))
	}

	h, err := os.Hostname()
	if err != nil {
		return "", err
	}

	if num > 1 {
		return fmt.Sprintf("hub for %s@%s %d", n, h, num), nil
	}

	return fmt.Sprintf("hub for %s@%s", n, h), nil
}
