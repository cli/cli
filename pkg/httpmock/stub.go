package httpmock

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

type Matcher func(req *http.Request) bool
type Responder func(req *http.Request) (*http.Response, error)

type Stub struct {
	matched   bool
	Matcher   Matcher
	Responder Responder
	exclude   bool
}

func MatchAny(*http.Request) bool {
	return true
}

// REST returns a matcher to a request for the HTTP method and URL escaped path p.
// For example, to match a GET request to `/api/v3/repos/octocat/hello-world/`
// use REST("GET", "api/v3/repos/octocat/hello-world")
// To match a GET request to `/user` use REST("GET", "user")
func REST(method, p string) Matcher {
	return func(req *http.Request) bool {
		if !strings.EqualFold(req.Method, method) {
			return false
		}
		return req.URL.EscapedPath() == "/"+p
	}
}

func GraphQL(q string) Matcher {
	re := regexp.MustCompile(q)

	return func(req *http.Request) bool {
		if !strings.EqualFold(req.Method, "POST") {
			return false
		}
		if req.URL.Path != "/graphql" && req.URL.Path != "/api/graphql" {
			return false
		}

		var bodyData struct {
			Query string
		}
		_ = decodeJSONBody(req, &bodyData)

		return re.MatchString(bodyData.Query)
	}
}

func GraphQLMutationMatcher(q string, cb func(map[string]interface{}) bool) Matcher {
	re := regexp.MustCompile(q)

	return func(req *http.Request) bool {
		if !strings.EqualFold(req.Method, "POST") {
			return false
		}
		if req.URL.Path != "/graphql" && req.URL.Path != "/api/graphql" {
			return false
		}

		var bodyData struct {
			Query     string
			Variables struct {
				Input map[string]interface{}
			}
		}
		_ = decodeJSONBody(req, &bodyData)

		if re.MatchString(bodyData.Query) {
			return cb(bodyData.Variables.Input)
		}

		return false
	}
}

func QueryMatcher(method string, path string, query url.Values) Matcher {
	return func(req *http.Request) bool {
		if !REST(method, path)(req) {
			return false
		}

		actualQuery := req.URL.Query()

		for param := range query {
			if !(actualQuery.Get(param) == query.Get(param)) {
				return false
			}
		}

		return true
	}
}

func readBody(req *http.Request) ([]byte, error) {
	bodyCopy := &bytes.Buffer{}
	r := io.TeeReader(req.Body, bodyCopy)
	req.Body = io.NopCloser(bodyCopy)
	return io.ReadAll(r)
}

func decodeJSONBody(req *http.Request, dest interface{}) error {
	b, err := readBody(req)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

func StringResponse(body string) Responder {
	return func(req *http.Request) (*http.Response, error) {
		return httpResponse(200, req, bytes.NewBufferString(body)), nil
	}
}

func WithHost(matcher Matcher, host string) Matcher {
	return func(req *http.Request) bool {
		if !strings.EqualFold(req.Host, host) {
			return false
		}
		return matcher(req)
	}
}

func WithHeader(responder Responder, header string, value string) Responder {
	return func(req *http.Request) (*http.Response, error) {
		resp, _ := responder(req)
		if resp.Header == nil {
			resp.Header = make(http.Header)
		}
		resp.Header.Set(header, value)
		return resp, nil
	}
}

func StatusStringResponse(status int, body string) Responder {
	return func(req *http.Request) (*http.Response, error) {
		return httpResponse(status, req, bytes.NewBufferString(body)), nil
	}
}

func JSONResponse(body interface{}) Responder {
	return func(req *http.Request) (*http.Response, error) {
		b, _ := json.Marshal(body)
		header := http.Header{
			"Content-Type": []string{"application/json"},
		}
		return httpResponseWithHeader(200, req, bytes.NewBuffer(b), header), nil
	}
}

func StatusJSONResponse(status int, body interface{}) Responder {
	return func(req *http.Request) (*http.Response, error) {
		b, _ := json.Marshal(body)
		header := http.Header{
			"Content-Type": []string{"application/json"},
		}
		return httpResponseWithHeader(status, req, bytes.NewBuffer(b), header), nil
	}
}

func FileResponse(filename string) Responder {
	return func(req *http.Request) (*http.Response, error) {
		f, err := os.Open(filename)
		if err != nil {
			return nil, err
		}
		return httpResponse(200, req, f), nil
	}
}

func RESTPayload(responseStatus int, responseBody string, cb func(payload map[string]interface{})) Responder {
	return func(req *http.Request) (*http.Response, error) {
		bodyData := make(map[string]interface{})
		err := decodeJSONBody(req, &bodyData)
		if err != nil {
			return nil, err
		}
		cb(bodyData)
		return httpResponse(responseStatus, req, bytes.NewBufferString(responseBody)), nil
	}
}

func GraphQLMutation(body string, cb func(map[string]interface{})) Responder {
	return func(req *http.Request) (*http.Response, error) {
		var bodyData struct {
			Variables struct {
				Input map[string]interface{}
			}
		}
		err := decodeJSONBody(req, &bodyData)
		if err != nil {
			return nil, err
		}
		cb(bodyData.Variables.Input)

		return httpResponse(200, req, bytes.NewBufferString(body)), nil
	}
}

func GraphQLQuery(body string, cb func(string, map[string]interface{})) Responder {
	return func(req *http.Request) (*http.Response, error) {
		var bodyData struct {
			Query     string
			Variables map[string]interface{}
		}
		err := decodeJSONBody(req, &bodyData)
		if err != nil {
			return nil, err
		}
		cb(bodyData.Query, bodyData.Variables)

		return httpResponse(200, req, bytes.NewBufferString(body)), nil
	}
}

// ScopesResponder returns a response with a 200 status code and the given OAuth scopes.
func ScopesResponder(scopes string) func(*http.Request) (*http.Response, error) {
	return StatusScopesResponder(http.StatusOK, scopes)
}

// StatusScopesResponder returns a response with the given status code and OAuth scopes.
func StatusScopesResponder(status int, scopes string) func(*http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Request:    req,
			Header: map[string][]string{
				"X-Oauth-Scopes": {scopes},
			},
			Body: io.NopCloser(bytes.NewBufferString("")),
		}, nil
	}
}

func httpResponse(status int, req *http.Request, body io.Reader) *http.Response {
	return httpResponseWithHeader(status, req, body, http.Header{})
}

func httpResponseWithHeader(status int, req *http.Request, body io.Reader, header http.Header) *http.Response {
	return &http.Response{
		StatusCode: status,
		Request:    req,
		Body:       io.NopCloser(body),
		Header:     header,
	}
}
