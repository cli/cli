package search

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/safeurl"
)

const (
	// GitHub API has a limit of 100 per page
	maxPerPage = 100
	orderKey   = "order"
	sortKey    = "sort"
)

var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)
var pageRE = regexp.MustCompile(`(\?|&)page=(\d*)`)

//go:generate moq -rm -out searcher_mock.go . Searcher
type Searcher interface {
	Code(Query) (CodeResult, error)
	Commits(Query) (CommitsResult, error)
	Repositories(Query) (RepositoriesResult, error)
	Issues(Query) (IssuesResult, error)
	URL(Query) string
}

type searcher struct {
	client   *http.Client
	detector fd.Detector
	host     string
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

func NewSearcher(client *http.Client, host string, detector fd.Detector) Searcher {
	return &searcher{
		client:   client,
		host:     host,
		detector: detector,
	}
}

func (s searcher) Code(query Query) (CodeResult, error) {
	result := CodeResult{}

	// We will request either the query limit if it's less than 1 page, or our max page size.
	// This number doesn't change to keep a valid offset.
	//
	// For example, say we want 150 items out of 500.
	// We request page #1 for 100 items and get items 0 to 99.
	// Then we request page #2 for 100 items, we get items 100 to 199 and only keep 100 to 149.
	// If we were to request page #2 for 50 items, we would instead get items 50 to 99.
	numItemsToRetrieve := query.Limit
	query.Limit = min(numItemsToRetrieve, maxPerPage)
	query.Page = 1

	for numItemsToRetrieve > 0 {
		page := CodeResult{}
		link, err := s.search(query, &page)
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

		query.Page = nextPage(link)
		if query.Page == 0 {
			break
		}
	}

	return result, nil
}

func (s searcher) Commits(query Query) (CommitsResult, error) {
	result := CommitsResult{}

	numItemsToRetrieve := query.Limit
	query.Limit = min(numItemsToRetrieve, maxPerPage)
	query.Page = 1

	for numItemsToRetrieve > 0 {
		page := CommitsResult{}
		link, err := s.search(query, &page)
		if err != nil {
			return result, err
		}

		numItemsToAdd := min(len(page.Items), numItemsToRetrieve)
		result.IncompleteResults = page.IncompleteResults
		result.Total = page.Total
		result.Items = append(result.Items, page.Items[:numItemsToAdd]...)
		numItemsToRetrieve = numItemsToRetrieve - numItemsToAdd

		query.Page = nextPage(link)
		if query.Page == 0 {
			break
		}
	}
	return result, nil
}

func (s searcher) Repositories(query Query) (RepositoriesResult, error) {
	result := RepositoriesResult{}

	numItemsToRetrieve := query.Limit
	query.Limit = min(numItemsToRetrieve, maxPerPage)
	query.Page = 1

	for numItemsToRetrieve > 0 {
		page := RepositoriesResult{}
		link, err := s.search(query, &page)
		if err != nil {
			return result, err
		}

		numItemsToAdd := min(len(page.Items), numItemsToRetrieve)
		result.IncompleteResults = page.IncompleteResults
		result.Total = page.Total
		result.Items = append(result.Items, page.Items[:numItemsToAdd]...)
		numItemsToRetrieve = numItemsToRetrieve - numItemsToAdd

		query.Page = nextPage(link)
		if query.Page == 0 {
			break
		}
	}
	return result, nil
}

func (s searcher) Issues(query Query) (IssuesResult, error) {
	result := IssuesResult{}

	// Semantic and hybrid searches use a separate, smaller rate-limit bucket and
	// are relevance-ranked, so bound fetching to a single page.
	singlePage := query.IssueSearchType == "semantic" || query.IssueSearchType == "hybrid"

	numItemsToRetrieve := query.Limit
	query.Limit = min(numItemsToRetrieve, maxPerPage)
	query.Page = 1
	for numItemsToRetrieve > 0 {
		page := IssuesResult{}
		link, err := s.search(query, &page)
		if err != nil {
			return result, err
		}

		numItemsToAdd := min(len(page.Items), numItemsToRetrieve)
		result.IncompleteResults = page.IncompleteResults
		result.Total = page.Total
		result.Items = append(result.Items, page.Items[:numItemsToAdd]...)
		numItemsToRetrieve = numItemsToRetrieve - numItemsToAdd

		if singlePage {
			break
		}

		query.Page = nextPage(link)
		if query.Page == 0 {
			break
		}
	}
	return result, nil
}

// search makes a single-page REST search request for code, commits, issues, prs, or repos,
// and returns the link header from response for further pagination calls. If the link header
// is not set on the response, empty string is returned.
//
// The result argument is populated with the following information:
//
// - Total: the number of search results matching the query, which may exceed the number of items returned
// - IncompleteResults: whether the search request exceeded search time limit, potentially being incomplete
// - Items: the actual matching search results, up to 100 max items per page
//
// For more information, see https://docs.github.com/en/rest/search/search?apiVersion=2022-11-28.
func (s searcher) search(query Query, result any) (string, error) {
	u, err := safeurl.JoinPath("search", string(query.Kind))
	if err != nil {
		return "", err
	}
	u.SetQuery("page", strconv.Itoa(query.Page))
	u.SetQuery("per_page", strconv.Itoa(query.Limit))

	if query.Kind == KindIssues {
		// TODO advancedIssueSearchCleanup
		// We won't need feature detection when GHES 3.17 support ends, since
		// the advanced issue search is the only available search backend for
		// issues.
		features, err := s.detector.SearchFeatures()
		if err != nil {
			return "", err
		}

		if !features.AdvancedIssueSearchAPI {
			u.SetQuery("q", query.StandardSearchString())
		} else {
			u.SetQuery("q", query.AdvancedIssueSearchString())

			// TODO advancedIssueSearchCleanup
			if features.AdvancedIssueSearchAPIOptIn {
				// Advanced syntax should be explicitly enabled
				u.SetQuery("advanced_search", "true")
			}
		}

		switch query.IssueSearchType {
		case "semantic":
			if !features.SemanticSearch {
				return "", fmt.Errorf("semantic search is not supported on this host: %s", s.host)
			}
			u.SetQuery("search_type", query.IssueSearchType)
		case "hybrid":
			if !features.HybridSearch {
				return "", fmt.Errorf("hybrid search is not supported on this host: %s", s.host)
			}
			u.SetQuery("search_type", query.IssueSearchType)
		}
	} else {
		u.SetQuery("q", query.StandardSearchString())
	}

	if query.Order != "" {
		u.SetQuery(orderKey, query.Order)
	}
	if query.Sort != "" {
		u.SetQuery(sortKey, query.Sort)
	}
	accept := "application/vnd.github.v3+json"
	if query.Kind == KindCode {
		accept = "application/vnd.github.text-match+json"
	}

	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, err := api.NewClientFromHTTP(s.client).Request(s.host, http.MethodGet, u.String(), nil,
		api.WithHeader("Accept", accept))
	if err != nil {
		// Search reports query errors in a shape of its own, so the generic API error is
		// translated back rather than surfaced directly.
		if apiErr, ok := errors.AsType[api.HTTPError](err); ok {
			return apiErr.Headers.Get("Link"), asSearchError(apiErr)
		}
		return "", err
	}
	defer resp.Body.Close()

	link := resp.Header.Get("Link")

	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(result)
	if err != nil {
		return link, err
	}
	return link, nil
}

// URL returns URL to the global search in web GUI (i.e. github.com/search).
func (s searcher) URL(query Query) string {
	path := fmt.Sprintf("https://%s/search", s.host)
	qs := url.Values{}
	qs.Set("type", query.Kind)

	// TODO advancedSearchFuture
	// Currently, the global search GUI does not support the advanced issue
	// search syntax (even for the issues/PRs tab on the sidebar). When the GUI
	// is updated, we can use feature detection, and, if available, use the
	// advanced search syntax.
	qs.Set("q", query.StandardSearchString())

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

// asSearchError converts an api.HTTPError into search's own error type, which formats
// invalid queries differently from the rest of the CLI.
func asSearchError(apiErr api.HTTPError) error {
	searchErr := httpError{
		Message:    apiErr.Message,
		RequestURL: apiErr.RequestURL,
		StatusCode: apiErr.StatusCode,
	}
	// A non-JSON response leaves Message empty, where this package used to report the status line.
	if searchErr.Message == "" {
		searchErr.Message = fmt.Sprintf("%d %s", apiErr.StatusCode, http.StatusText(apiErr.StatusCode))
	}
	for _, item := range apiErr.Errors {
		searchErr.Errors = append(searchErr.Errors, httpErrorItem{
			Code:     item.Code,
			Field:    item.Field,
			Message:  item.Message,
			Resource: item.Resource,
		})
	}
	return searchErr
}

// nextPage extracts the next page number from an API response's link header. if
// the provided link header is empty or there is no next page, zero is returned.
//
// See API [docs] on pagination for more information.
//
// [docs]: https://docs.github.com/en/rest/using-the-rest-api/using-pagination-in-the-rest-api
func nextPage(link string) (page int) {
	// When using pagination, responses get a "Link" field in their header.
	// When a next page is available, "Link" contains a link to the next page
	// tagged with rel="next".
	for _, m := range linkRE.FindAllStringSubmatch(link, -1) {
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
