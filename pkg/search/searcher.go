package search

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/internal/ghinstance"
)

const (
	maxPerPage = 100
	orderKey   = "order"
	sortKey    = "sort"
)

var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)
var pageRE = regexp.MustCompile(`(\?|&)page=(\d*)`)
var jsonTypeRE = regexp.MustCompile(`[/+]json($|;)`)

//go:generate moq -rm -out searcher_mock.go . Searcher
type Searcher interface {
	Commits(Query) (CommitsResult, error)
	Repositories(Query) (RepositoriesResult, error)
	Issues(Query) (IssuesResult, error)
	URL(Query) string
}

type searcher struct {
	client *http.Client
	host   string
}

type httpError struct {
	Errors     []httpErrorItem
	Message    string
	RequestURL *url.URL
	StatusCode int
}

type httpErrorItem struct {
	Code     string
	Field    string
	Message  string
	Resource string
}

func NewSearcher(client *http.Client, host string) Searcher {
	return &searcher{
		client: client,
		host:   host,
	}
}

func (s searcher) Commits(query Query) (CommitsResult, error) {
	result := CommitsResult{}
	toRetrieve := query.Limit
	var resp *http.Response
	var err error
	for toRetrieve > 0 {
		query.Limit = min(toRetrieve, maxPerPage)
		query.Page = nextPage(resp)
		if query.Page == 0 {
			break
		}
		page := CommitsResult{}
		resp, err = s.search(query, &page)
		if err != nil {
			return result, err
		}
		result.IncompleteResults = page.IncompleteResults
		result.Total = page.Total
		result.Items = append(result.Items, page.Items...)
		toRetrieve = toRetrieve - len(page.Items)
	}
	return result, nil
}

func (s searcher) Repositories(query Query) (RepositoriesResult, error) {
	result := RepositoriesResult{}
	toRetrieve := query.Limit
	var resp *http.Response
	var err error
	for toRetrieve > 0 {
		query.Limit = min(toRetrieve, maxPerPage)
		query.Page = nextPage(resp)
		if query.Page == 0 {
			break
		}
		page := RepositoriesResult{}
		resp, err = s.search(query, &page)
		if err != nil {
			return result, err
		}
		result.IncompleteResults = page.IncompleteResults
		result.Total = page.Total
		result.Items = append(result.Items, page.Items...)
		toRetrieve = toRetrieve - len(page.Items)
	}
	return result, nil
}

func (s searcher) Issues(query Query) (IssuesResult, error) {
	result := IssuesResult{}
	toRetrieve := query.Limit
	var resp *http.Response
	var err error
	for toRetrieve > 0 {
		query.Limit = min(toRetrieve, maxPerPage)
		query.Page = nextPage(resp)
		if query.Page == 0 {
			break
		}
		page := IssuesResult{}
		resp, err = s.search(query, &page)
		if err != nil {
			return result, err
		}
		result.IncompleteResults = page.IncompleteResults
		result.Total = page.Total
		result.Items = append(result.Items, page.Items...)
		toRetrieve = toRetrieve - len(page.Items)
	}
	return result, nil
}

func (s searcher) search(query Query, result interface{}) (*http.Response, error) {
	path := fmt.Sprintf("%ssearch/%s", ghinstance.RESTPrefix(s.host), query.Kind)
	qs := url.Values{}
	qs.Set("page", strconv.Itoa(query.Page))
	qs.Set("per_page", strconv.Itoa(query.Limit))
	qs.Set("q", query.String())
	if query.Order != "" {
		qs.Set(orderKey, query.Order)
	}
	if query.Sort != "" {
		qs.Set(sortKey, query.Sort)
	}
	url := fmt.Sprintf("%s?%s", path, qs.Encode())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return resp, handleHTTPError(resp)
	}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(result)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

func (s searcher) URL(query Query) string {
	path := fmt.Sprintf("https://%s/search", s.host)
	qs := url.Values{}
	qs.Set("type", query.Kind)
	qs.Set("q", query.String())
	if query.Order != "" {
		qs.Set(orderKey, query.Order)
	}
	if query.Sort != "" {
		qs.Set(sortKey, query.Sort)
	}
	url := fmt.Sprintf("%s?%s", path, qs.Encode())
	return url
}

func (err httpError) Error() string {
	if err.StatusCode != 422 || len(err.Errors) == 0 {
		return fmt.Sprintf("HTTP %d: %s (%s)", err.StatusCode, err.Message, err.RequestURL)
	}
	query := strings.TrimSpace(err.RequestURL.Query().Get("q"))
	return fmt.Sprintf("Invalid search query %q.\n%s", query, err.Errors[0].Message)
}

func handleHTTPError(resp *http.Response) error {
	httpError := httpError{
		RequestURL: resp.Request.URL,
		StatusCode: resp.StatusCode,
	}
	if !jsonTypeRE.MatchString(resp.Header.Get("Content-Type")) {
		httpError.Message = resp.Status
		return httpError
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, &httpError); err != nil {
		return err
	}
	return httpError
}

func nextPage(resp *http.Response) (page int) {
	if resp == nil {
		return 1
	}
	for _, m := range linkRE.FindAllStringSubmatch(resp.Header.Get("Link"), -1) {
		if !(len(m) > 2 && m[2] == "next") {
			continue
		}
		p := pageRE.FindStringSubmatch(m[1])
		if len(p) == 3 {
			i, err := strconv.Atoi(p[2])
			if err == nil {
				return i
			}
		}
	}
	return 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
