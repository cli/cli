package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/github/gh-cli/version"
)

const (
	GitHubHost  string = "github.com"
	OAuthAppURL string = "https://hub.github.com/"
)

var UserAgent = "Hub " + version.Version

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

func (client *Client) FetchPullRequests(project *Project, filterParams map[string]interface{}, limit int, filter func(*PullRequest) bool) (pulls []PullRequest, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	path := fmt.Sprintf("repos/%s/%s/pulls?per_page=%d", project.Owner, project.Name, perPage(limit, 100))
	if filterParams != nil {
		path = addQuery(path, filterParams)
	}

	pulls = []PullRequest{}
	var res *simpleResponse

	for path != "" {
		res, err = api.GetFile(path, draftsType)
		if err = checkStatus(200, "fetching pull requests", res, err); err != nil {
			return
		}
		path = res.Link("next")

		pullsPage := []PullRequest{}
		if err = res.Unmarshal(&pullsPage); err != nil {
			return
		}
		for _, pr := range pullsPage {
			if filter == nil || filter(&pr) {
				pulls = append(pulls, pr)
				if limit > 0 && len(pulls) == limit {
					path = ""
					break
				}
			}
		}
	}

	return
}

func (client *Client) PullRequest(project *Project, id string) (pr *PullRequest, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.Get(fmt.Sprintf("repos/%s/%s/pulls/%s", project.Owner, project.Name, id))
	if err = checkStatus(200, "getting pull request", res, err); err != nil {
		return
	}

	pr = &PullRequest{}
	err = res.Unmarshal(pr)

	return
}

func (client *Client) CreatePullRequest(project *Project, params map[string]interface{}) (pr *PullRequest, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.PostJSONPreview(fmt.Sprintf("repos/%s/%s/pulls", project.Owner, project.Name), params, draftsType)
	if err = checkStatus(201, "creating pull request", res, err); err != nil {
		if res != nil && res.StatusCode == 404 {
			projectUrl := strings.SplitN(project.WebURL("", "", ""), "://", 2)[1]
			err = fmt.Errorf("%s\nAre you sure that %s exists?", err, projectUrl)
		}
		return
	}

	pr = &PullRequest{}
	err = res.Unmarshal(pr)

	return
}

func (client *Client) UpdatePullRequest(pr *PullRequest, params map[string]interface{}) (updatedPullRequest *PullRequest, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.PatchJSON(pr.ApiUrl, params)
	if err = checkStatus(200, "updating pull request", res, err); err != nil {
		return
	}

	updatedPullRequest = &PullRequest{}
	err = res.Unmarshal(updatedPullRequest)
	return
}

func (client *Client) RequestReview(project *Project, prNumber int, params map[string]interface{}) (err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.PostJSON(fmt.Sprintf("repos/%s/%s/pulls/%d/requested_reviewers", project.Owner, project.Name, prNumber), params)
	if err = checkStatus(201, "requesting reviewer", res, err); err != nil {
		return
	}

	res.Body.Close()
	return
}

func (client *Client) Repository(project *Project) (repo *Repository, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.Get(fmt.Sprintf("repos/%s/%s", project.Owner, project.Name))
	if err = checkStatus(200, "getting repository info", res, err); err != nil {
		return
	}

	repo = &Repository{}
	err = res.Unmarshal(&repo)
	return
}

type Repository struct {
	Name          string                 `json:"name"`
	FullName      string                 `json:"full_name"`
	Parent        *Repository            `json:"parent"`
	Owner         *User                  `json:"owner"`
	Private       bool                   `json:"private"`
	HasWiki       bool                   `json:"has_wiki"`
	Permissions   *RepositoryPermissions `json:"permissions"`
	HtmlUrl       string                 `json:"html_url"`
	DefaultBranch string                 `json:"default_branch"`
}

type RepositoryPermissions struct {
	Admin bool `json:"admin"`
	Push  bool `json:"push"`
	Pull  bool `json:"pull"`
}

type Issue struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	User   *User  `json:"user"`

	PullRequest *PullRequest     `json:"pull_request"`
	Head        *PullRequestSpec `json:"head"`
	Base        *PullRequestSpec `json:"base"`

	MergeCommitSha      string `json:"merge_commit_sha"`
	MaintainerCanModify bool   `json:"maintainer_can_modify"`
	Draft               bool   `json:"draft"`

	Comments  int          `json:"comments"`
	Labels    []IssueLabel `json:"labels"`
	Assignees []User       `json:"assignees"`
	Milestone *Milestone   `json:"milestone"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	MergedAt  time.Time    `json:"merged_at"`

	RequestedReviewers []User `json:"requested_reviewers"`
	RequestedTeams     []Team `json:"requested_teams"`

	ApiUrl  string `json:"url"`
	HtmlUrl string `json:"html_url"`

	ClosedBy *User `json:"closed_by"`
}

type PullRequest Issue

type PullRequestSpec struct {
	Label string      `json:"label"`
	Ref   string      `json:"ref"`
	Sha   string      `json:"sha"`
	Repo  *Repository `json:"repo"`
}

func (pr *PullRequest) IsSameRepo() bool {
	return pr.Head != nil && pr.Head.Repo != nil &&
		pr.Head.Repo.Name == pr.Base.Repo.Name &&
		pr.Head.Repo.Owner.Login == pr.Base.Repo.Owner.Login
}

func (pr *PullRequest) HasRequestedReviewer(name string) bool {
	for _, user := range pr.RequestedReviewers {
		if strings.EqualFold(user.Login, name) {
			return true
		}
	}
	return false
}

func (pr *PullRequest) HasRequestedTeam(name string) bool {
	for _, team := range pr.RequestedTeams {
		if strings.EqualFold(team.Slug, name) {
			return true
		}
	}
	return false
}

type IssueLabel struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type User struct {
	Login string `json:"login"`
}

type Team struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Milestone struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

func (client *Client) GenericAPIRequest(method, path string, data interface{}, headers map[string]string, ttl int) (*simpleResponse, error) {
	api, err := client.simpleApi()
	if err != nil {
		return nil, err
	}
	api.CacheTTL = ttl

	var body io.Reader
	switch d := data.(type) {
	case map[string]interface{}:
		if method == "GET" {
			path = addQuery(path, d)
		} else if len(d) > 0 {
			json, err := json.Marshal(d)
			if err != nil {
				return nil, err
			}
			body = bytes.NewBuffer(json)
		}
	case io.Reader:
		body = d
	}

	return api.performRequest(method, path, body, func(req *http.Request) {
		if body != nil {
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	})
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

func perPage(limit, max int) int {
	if limit > 0 {
		limit = limit + (limit / 2)
		if limit < max {
			return limit
		}
	}
	return max
}

func addQuery(path string, params map[string]interface{}) string {
	if len(params) == 0 {
		return path
	}

	query := url.Values{}
	for key, value := range params {
		switch v := value.(type) {
		case string:
			query.Add(key, v)
		case nil:
			query.Add(key, "")
		case int:
			query.Add(key, fmt.Sprintf("%d", v))
		case bool:
			query.Add(key, fmt.Sprintf("%v", v))
		}
	}

	sep := "?"
	if strings.Contains(path, sep) {
		sep = "&"
	}
	return path + sep + query.Encode()
}
