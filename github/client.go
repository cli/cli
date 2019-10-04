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
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/github/gh/version"
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

func (client *Client) PullRequestPatch(project *Project, id string) (patch io.ReadCloser, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.GetFile(fmt.Sprintf("repos/%s/%s/pulls/%s", project.Owner, project.Name, id), patchMediaType)
	if err = checkStatus(200, "getting pull request patch", res, err); err != nil {
		return
	}

	return res.Body, nil
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

func (client *Client) CommitPatch(project *Project, sha string) (patch io.ReadCloser, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.GetFile(fmt.Sprintf("repos/%s/%s/commits/%s", project.Owner, project.Name, sha), patchMediaType)
	if err = checkStatus(200, "getting commit patch", res, err); err != nil {
		return
	}

	return res.Body, nil
}

type Gist struct {
	Files map[string]GistFile `json:"files"`
}
type GistFile struct {
	RawUrl string `json:"raw_url"`
}

func (client *Client) GistPatch(id string) (patch io.ReadCloser, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.Get(fmt.Sprintf("gists/%s", id))
	if err = checkStatus(200, "getting gist patch", res, err); err != nil {
		return
	}

	gist := Gist{}
	if err = res.Unmarshal(&gist); err != nil {
		return
	}
	rawUrl := ""
	for _, file := range gist.Files {
		rawUrl = file.RawUrl
		break
	}

	res, err = api.GetFile(rawUrl, textMediaType)
	if err = checkStatus(200, "getting gist patch", res, err); err != nil {
		return
	}

	return res.Body, nil
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

func (client *Client) CreateRepository(project *Project, description, homepage string, isPrivate bool) (repo *Repository, err error) {
	repoURL := "user/repos"
	if project.Owner != client.Host.User {
		repoURL = fmt.Sprintf("orgs/%s/repos", project.Owner)
	}

	params := map[string]interface{}{
		"name":        project.Name,
		"description": description,
		"homepage":    homepage,
		"private":     isPrivate,
	}

	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.PostJSON(repoURL, params)
	if err = checkStatus(201, "creating repository", res, err); err != nil {
		return
	}

	repo = &Repository{}
	err = res.Unmarshal(repo)
	return
}

func (client *Client) DeleteRepository(project *Project) error {
	api, err := client.simpleApi()
	if err != nil {
		return err
	}

	repoURL := fmt.Sprintf("repos/%s/%s", project.Owner, project.Name)
	res, err := api.Delete(repoURL)
	return checkStatus(204, "deleting repository", res, err)
}

type Release struct {
	Name            string         `json:"name"`
	TagName         string         `json:"tag_name"`
	TargetCommitish string         `json:"target_commitish"`
	Body            string         `json:"body"`
	Draft           bool           `json:"draft"`
	Prerelease      bool           `json:"prerelease"`
	Assets          []ReleaseAsset `json:"assets"`
	TarballUrl      string         `json:"tarball_url"`
	ZipballUrl      string         `json:"zipball_url"`
	HtmlUrl         string         `json:"html_url"`
	UploadUrl       string         `json:"upload_url"`
	ApiUrl          string         `json:"url"`
	CreatedAt       time.Time      `json:"created_at"`
	PublishedAt     time.Time      `json:"published_at"`
}

type ReleaseAsset struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	DownloadUrl string `json:"browser_download_url"`
	ApiUrl      string `json:"url"`
}

func (client *Client) FetchReleases(project *Project, limit int, filter func(*Release) bool) (releases []Release, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	path := fmt.Sprintf("repos/%s/%s/releases?per_page=%d", project.Owner, project.Name, perPage(limit, 100))

	releases = []Release{}
	var res *simpleResponse

	for path != "" {
		res, err = api.Get(path)
		if err = checkStatus(200, "fetching releases", res, err); err != nil {
			return
		}
		path = res.Link("next")

		releasesPage := []Release{}
		if err = res.Unmarshal(&releasesPage); err != nil {
			return
		}
		for _, release := range releasesPage {
			if filter == nil || filter(&release) {
				releases = append(releases, release)
				if limit > 0 && len(releases) == limit {
					path = ""
					break
				}
			}
		}
	}

	return
}

func (client *Client) FetchRelease(project *Project, tagName string) (*Release, error) {
	releases, err := client.FetchReleases(project, 100, func(release *Release) bool {
		return release.TagName == tagName
	})

	if err == nil {
		if len(releases) < 1 {
			return nil, fmt.Errorf("Unable to find release with tag name `%s'", tagName)
		} else {
			return &releases[0], nil
		}
	} else {
		return nil, err
	}
}

func (client *Client) CreateRelease(project *Project, releaseParams *Release) (release *Release, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.PostJSON(fmt.Sprintf("repos/%s/%s/releases", project.Owner, project.Name), releaseParams)
	if err = checkStatus(201, "creating release", res, err); err != nil {
		return
	}

	release = &Release{}
	err = res.Unmarshal(release)
	return
}

func (client *Client) EditRelease(release *Release, releaseParams map[string]interface{}) (updatedRelease *Release, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.PatchJSON(release.ApiUrl, releaseParams)
	if err = checkStatus(200, "editing release", res, err); err != nil {
		return
	}

	updatedRelease = &Release{}
	err = res.Unmarshal(updatedRelease)
	return
}

func (client *Client) DeleteRelease(release *Release) (err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.Delete(release.ApiUrl)
	if err = checkStatus(204, "deleting release", res, err); err != nil {
		return
	}

	return
}

func (client *Client) UploadReleaseAsset(release *Release, filename, label string) (asset *ReleaseAsset, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	parts := strings.SplitN(release.UploadUrl, "{", 2)
	uploadUrl := parts[0]
	uploadUrl += "?name=" + url.QueryEscape(filepath.Base(filename))
	if label != "" {
		uploadUrl += "&label=" + url.QueryEscape(label)
	}

	res, err := api.PostFile(uploadUrl, filename)
	if err = checkStatus(201, "uploading release asset", res, err); err != nil {
		return
	}

	asset = &ReleaseAsset{}
	err = res.Unmarshal(asset)
	return
}

func (client *Client) DeleteReleaseAsset(asset *ReleaseAsset) (err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.Delete(asset.ApiUrl)
	err = checkStatus(204, "deleting release asset", res, err)

	return
}

func (client *Client) DownloadReleaseAsset(url string) (asset io.ReadCloser, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	resp, err := api.GetFile(url, "application/octet-stream")
	if err = checkStatus(200, "downloading asset", resp, err); err != nil {
		return
	}

	return resp.Body, err
}

type CIStatusResponse struct {
	State    string     `json:"state"`
	Statuses []CIStatus `json:"statuses"`
}

type CIStatus struct {
	State     string `json:"state"`
	Context   string `json:"context"`
	TargetUrl string `json:"target_url"`
}

type CheckRunsResponse struct {
	CheckRuns []CheckRun `json:"check_runs"`
}

type CheckRun struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Name       string `json:"name"`
	HtmlUrl    string `json:"html_url"`
}

func (client *Client) FetchCIStatus(project *Project, sha string) (status *CIStatusResponse, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.Get(fmt.Sprintf("repos/%s/%s/commits/%s/status", project.Owner, project.Name, sha))
	if err = checkStatus(200, "fetching statuses", res, err); err != nil {
		return
	}

	status = &CIStatusResponse{}
	if err = res.Unmarshal(status); err != nil {
		return
	}

	sortStatuses := func() {
		sort.Slice(status.Statuses, func(a, b int) bool {
			sA := status.Statuses[a]
			sB := status.Statuses[b]
			cmp := strings.Compare(strings.ToLower(sA.Context), strings.ToLower(sB.Context))
			if cmp == 0 {
				return strings.Compare(sA.TargetUrl, sB.TargetUrl) < 0
			} else {
				return cmp < 0
			}
		})
	}
	sortStatuses()

	res, err = api.GetFile(fmt.Sprintf("repos/%s/%s/commits/%s/check-runs", project.Owner, project.Name, sha), checksType)
	if err == nil && (res.StatusCode == 403 || res.StatusCode == 404 || res.StatusCode == 422) {
		return
	}
	if err = checkStatus(200, "fetching checks", res, err); err != nil {
		return
	}

	checks := &CheckRunsResponse{}
	if err = res.Unmarshal(checks); err != nil {
		return
	}

	for _, checkRun := range checks.CheckRuns {
		state := "pending"
		if checkRun.Status == "completed" {
			state = checkRun.Conclusion
		}
		checkStatus := CIStatus{
			State:     state,
			Context:   checkRun.Name,
			TargetUrl: checkRun.HtmlUrl,
		}
		status.Statuses = append(status.Statuses, checkStatus)
	}

	sortStatuses()

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

func (client *Client) ForkRepository(project *Project, params map[string]interface{}) (repo *Repository, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.PostJSON(fmt.Sprintf("repos/%s/%s/forks", project.Owner, project.Name), params)
	if err = checkStatus(202, "creating fork", res, err); err != nil {
		return
	}

	repo = &Repository{}
	err = res.Unmarshal(repo)

	return
}

type Comment struct {
	Id        int       `json:"id"`
	Body      string    `json:"body"`
	User      *User     `json:"user"`
	CreatedAt time.Time `json:"created_at"`
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

func (client *Client) FetchIssues(project *Project, filterParams map[string]interface{}, limit int, filter func(*Issue) bool) (issues []Issue, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	path := fmt.Sprintf("repos/%s/%s/issues?per_page=%d", project.Owner, project.Name, perPage(limit, 100))
	if filterParams != nil {
		path = addQuery(path, filterParams)
	}

	issues = []Issue{}
	var res *simpleResponse

	for path != "" {
		res, err = api.Get(path)
		if err = checkStatus(200, "fetching issues", res, err); err != nil {
			return
		}
		path = res.Link("next")

		issuesPage := []Issue{}
		if err = res.Unmarshal(&issuesPage); err != nil {
			return
		}
		for _, issue := range issuesPage {
			if filter == nil || filter(&issue) {
				issues = append(issues, issue)
				if limit > 0 && len(issues) == limit {
					path = ""
					break
				}
			}
		}
	}

	return
}

func (client *Client) FetchIssue(project *Project, number string) (issue *Issue, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.Get(fmt.Sprintf("repos/%s/%s/issues/%s", project.Owner, project.Name, number))
	if err = checkStatus(200, "fetching issue", res, err); err != nil {
		return nil, err
	}

	issue = &Issue{}
	err = res.Unmarshal(issue)
	return
}

func (client *Client) FetchComments(project *Project, number string) (comments []Comment, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.Get(fmt.Sprintf("repos/%s/%s/issues/%s/comments", project.Owner, project.Name, number))
	if err = checkStatus(200, "fetching comments for issue", res, err); err != nil {
		return nil, err
	}

	comments = []Comment{}
	err = res.Unmarshal(&comments)
	return
}

func (client *Client) CreateIssue(project *Project, params interface{}) (issue *Issue, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.PostJSON(fmt.Sprintf("repos/%s/%s/issues", project.Owner, project.Name), params)
	if err = checkStatus(201, "creating issue", res, err); err != nil {
		return
	}

	issue = &Issue{}
	err = res.Unmarshal(issue)
	return
}

func (client *Client) UpdateIssue(project *Project, issueNumber int, params map[string]interface{}) (err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	res, err := api.PatchJSON(fmt.Sprintf("repos/%s/%s/issues/%d", project.Owner, project.Name, issueNumber), params)
	if err = checkStatus(200, "updating issue", res, err); err != nil {
		return
	}

	res.Body.Close()
	return
}

type sortedLabels []IssueLabel

func (s sortedLabels) Len() int {
	return len(s)
}
func (s sortedLabels) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s sortedLabels) Less(i, j int) bool {
	return strings.Compare(strings.ToLower(s[i].Name), strings.ToLower(s[j].Name)) < 0
}

func (client *Client) FetchLabels(project *Project) (labels []IssueLabel, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	path := fmt.Sprintf("repos/%s/%s/labels?per_page=100", project.Owner, project.Name)

	labels = []IssueLabel{}
	var res *simpleResponse

	for path != "" {
		res, err = api.Get(path)
		if err = checkStatus(200, "fetching labels", res, err); err != nil {
			return
		}
		path = res.Link("next")

		labelsPage := []IssueLabel{}
		if err = res.Unmarshal(&labelsPage); err != nil {
			return
		}
		labels = append(labels, labelsPage...)
	}

	sort.Sort(sortedLabels(labels))

	return
}

func (client *Client) FetchMilestones(project *Project) (milestones []Milestone, err error) {
	api, err := client.simpleApi()
	if err != nil {
		return
	}

	path := fmt.Sprintf("repos/%s/%s/milestones?per_page=100", project.Owner, project.Name)

	milestones = []Milestone{}
	var res *simpleResponse

	for path != "" {
		res, err = api.Get(path)
		if err = checkStatus(200, "fetching milestones", res, err); err != nil {
			return
		}
		path = res.Link("next")

		milestonesPage := []Milestone{}
		if err = res.Unmarshal(&milestonesPage); err != nil {
			return
		}
		milestones = append(milestones, milestonesPage...)
	}

	return
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
