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
	// GitHub API has a limit of 100 per page
	maxPerPage = 100
	orderKey   = "order"
	sortKey    = "sort"
)

var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)
var pageRE = regexp.MustCompile(`(\?|&)page=(\d*)`)
var jsonTypeRE = regexp.MustCompile(`[/+]json($|;)`)

//go:generate moq -rm -out searcher_mock.go . Searcher
type Searcher interface {
	Code(Query) (CodeResult, error)
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

func (s searcher) Code(query Query) (CodeResult, error) {
	result := CodeResult{}

	var resp *http.Response
	var err error

	// We will request either the query limit if it's less than 1 page, or our max page size.
	// This number doesn't change to keep a valid offset.
	//
	// For example, say we want 150 items out of 500.
	// We request page #1 for 100 items and get items 0 to 99.
	// Then we request page #2 for 100 items, we get items 100 to 199 and only keep 100 to 149.
	// If we were to request page #2 for 50 items, we would instead get items 50 to 99.
	numItemsToRetrieve := query.Limit
	query.Limit = min(numItemsToRetrieve, maxPerPage)

	for numItemsToRetrieve > 0 {
		query.Page = nextPage(resp)
		if query.Page == 0 {
			break
		}

		page := CodeResult{}
		resp, err = s.search(query, &page)
		if err != nil {
			return result, err
		}

		// If we're going to reach the requested limit, only add that many items,
		// otherwise add all the results.
		numItemsToAdd := min(len(page.Items), numItemsToRetrieve)
		result.IncompleteResults = page.IncompleteResults
		// The API returns how many items match the query in every response.
		// With the example above, this would be 500.
		result.Total = page.Total
		result.Items = append(result.Items, page.Items[:numItemsToAdd]...)
		numItemsToRetrieve = numItemsToRetrieve - numItemsToAdd
	}

	return result, nil
}

func (s searcher) Commits(query Query) (CommitsResult, error) {
	result := CommitsResult{}

	var resp *http.Response
	var err error

	numItemsToRetrieve := query.Limit
	query.Limit = min(numItemsToRetrieve, maxPerPage)

	for numItemsToRetrieve > 0 {
		query.Page = nextPage(resp)
		if query.Page == 0 {
			break
		}

		page := CommitsResult{}
		resp, err = s.search(query, &page)
		if err != nil {
			return result, err
		}

		numItemsToAdd := min(len(page.Items), numItemsToRetrieve)
		result.IncompleteResults = page.IncompleteResults
		result.Total = page.Total
		result.Items = append(result.Items, page.Items[:numItemsToAdd]...)
		numItemsToRetrieve = numItemsToRetrieve - numItemsToAdd
	}
	return result, nil
}

func (s searcher) Repositories(query Query) (RepositoriesResult, error) {
	result := RepositoriesResult{}

	var resp *http.Response
	var err error

	numItemsToRetrieve := query.Limit
	query.Limit = min(numItemsToRetrieve, maxPerPage)

	for numItemsToRetrieve > 0 {
		query.Page = nextPage(resp)
		if query.Page == 0 {
			break
		}

		page := RepositoriesResult{}
		resp, err = s.search(query, &page)
		if err != nil {
			return result, err
		}

		numItemsToAdd := min(len(page.Items), numItemsToRetrieve)
		result.IncompleteResults = page.IncompleteResults
		result.Total = page.Total
		result.Items = append(result.Items, page.Items[:numItemsToAdd]...)
		numItemsToRetrieve = numItemsToRetrieve - numItemsToAdd
	}
	return result, nil
}

func (s searcher) Issues(query Query) (IssuesResult, error) {
	result := IssuesResult{}

	var resp *http.Response
	var err error

	numItemsToRetrieve := query.Limit
	query.Limit = min(numItemsToRetrieve, maxPerPage)

	for numItemsToRetrieve > 0 {
		query.Page = nextPage(resp)
		if query.Page == 0 {
			break
		}

		page := IssuesResult{}
		resp, err = s.search(query, &page)
		if err != nil {
			return result, err
		}

		numItemsToAdd := min(len(page.Items), numItemsToRetrieve)
		result.IncompleteResults = page.IncompleteResults
		result.Total = page.Total
		result.Items = append(result.Items, page.Items[:numItemsToAdd]...)
		numItemsToRetrieve = numItemsToRetrieve - numItemsToAdd
	}
	return result, nil
}

// search makes a single-page REST search request for code, commits, issues, prs, or repos.
//
// The result argument is populated with the following information:
//
// - Total: the number of search results matching the query, which may exceed the number of items returned
// - IncompleteResults: whether the search request exceeded search time limit, potentially being incomplete
// - Items: the actual matching search results, up to 100 max items per page
//
// For more information, see https://docs.github.com/en/rest/search/search?apiVersion=2022-11-28.
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
	if query.Kind == KindCode {
		req.Header.Set("Accept", "application/vnd.github.text-match+json")
	}

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

// https://docs.github.com/en/rest/using-the-rest-api/using-pagination-in-the-rest-api
func nextPage(resp *http.Response) (page int) {
	if resp == nil {
		return 1
	}

	// When using pagination, responses get a "Link" field in their header.
	// When a next page is available, "Link" contains a link to the next page
	// tagged with rel="next".
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
