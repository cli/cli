package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"github.com/henvic/httpretty"
)

// ClientOption represents an argument to NewClient
type ClientOption = func(http.RoundTripper) http.RoundTripper

// NewClient initializes a Client
func NewClient(opts ...ClientOption) *Client {
	tr := http.DefaultTransport
	for _, opt := range opts {
		tr = opt(tr)
	}
	http := &http.Client{Transport: tr}
	client := &Client{http: http}
	return client
}

// AddHeader turns a RoundTripper into one that adds a request header
func AddHeader(name, value string) ClientOption {
	return func(tr http.RoundTripper) http.RoundTripper {
		return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
			req.Header.Add(name, value)
			return tr.RoundTrip(req)
		}}
	}
}

// VerboseLog enables request/response logging within a RoundTripper
func VerboseLog(out io.Writer, logTraffic bool, colorize bool) ClientOption {
	logger := &httpretty.Logger{
		Time:           true,
		TLS:            false,
		Colors:         colorize,
		RequestHeader:  logTraffic,
		RequestBody:    logTraffic,
		ResponseHeader: logTraffic,
		ResponseBody:   logTraffic,
		Formatters:     []httpretty.Formatter{&httpretty.JSONFormatter{}},
	}
	logger.SetOutput(out)
	logger.SetBodyFilter(func(h http.Header) (skip bool, err error) {
		return !inspectableMIMEType(h.Get("Content-Type")), nil
	})
	return logger.RoundTripper
}

// ReplaceTripper substitutes the underlying RoundTripper with a custom one
func ReplaceTripper(tr http.RoundTripper) ClientOption {
	return func(http.RoundTripper) http.RoundTripper {
		return tr
	}
}

type funcTripper struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (tr funcTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return tr.roundTrip(req)
}

// Client facilitates making HTTP requests to the GitHub API
type Client struct {
	http *http.Client
}

type graphQLResponse struct {
	Data   interface{}
	Errors []GraphQLError
}

// GraphQLError is a single error returned in a GraphQL response
type GraphQLError struct {
	Type    string
	Path    []string
	Message string
}

// GraphQLErrorResponse contains errors returned in a GraphQL response
type GraphQLErrorResponse struct {
	Errors []GraphQLError
}

func (gr GraphQLErrorResponse) Error() string {
	errorMessages := make([]string, 0, len(gr.Errors))
	for _, e := range gr.Errors {
		errorMessages = append(errorMessages, e.Message)
	}
	return fmt.Sprintf("graphql error: '%s'", strings.Join(errorMessages, ", "))
}

// GraphQL performs a GraphQL request and parses the response
func (c Client) GraphQL(query string, variables map[string]interface{}, data interface{}) error {
	url := "https://api.github.com/graphql"
	reqBody, err := json.Marshal(map[string]interface{}{"query": query, "variables": variables})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return handleResponse(resp, data)
}

// REST performs a REST request and parses the response.
func (c Client) REST(method string, p string, body io.Reader, data interface{}) error {
	url := "https://api.github.com/" + p
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return handleHTTPError(resp)
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(b, &data)
	if err != nil {
		return err
	}

	return nil
}

func handleResponse(resp *http.Response, data interface{}) error {
	success := resp.StatusCode >= 200 && resp.StatusCode < 300

	if !success {
		return handleHTTPError(resp)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	gr := &graphQLResponse{Data: data}
	err = json.Unmarshal(body, &gr)
	if err != nil {
		return err
	}

	if len(gr.Errors) > 0 {
		return &GraphQLErrorResponse{Errors: gr.Errors}
	}
	return nil
}

func handleHTTPError(resp *http.Response) error {
	var message string
	var parsedBody struct {
		Message string `json:"message"`
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, &parsedBody)
	if err != nil {
		message = string(body)
	} else {
		message = parsedBody.Message
	}

	return fmt.Errorf("http error, '%s' failed (%d): '%s'", resp.Request.URL, resp.StatusCode, message)
}

var jsonTypeRE = regexp.MustCompile(`[/+]json($|;)`)

func inspectableMIMEType(t string) bool {
	return strings.HasPrefix(t, "text/") || jsonTypeRE.MatchString(t)
}
