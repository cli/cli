package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
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
func VerboseLog(out io.Writer) ClientOption {
	return func(tr http.RoundTripper) http.RoundTripper {
		return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
			fmt.Fprintf(out, "> %s %s\n", req.Method, req.URL.RequestURI())
			res, err := tr.RoundTrip(req)
			if err == nil {
				fmt.Fprintf(out, "< HTTP %s\n", res.Status)
			}
			return res, err
		}}
	}
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
	Errors []struct {
		Message string
	}
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
		errorMessages := gr.Errors[0].Message
		for _, e := range gr.Errors[1:] {
			errorMessages += ", " + e.Message
		}
		return fmt.Errorf("graphql error: '%s'", errorMessages)
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
