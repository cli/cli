package httpmock

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
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
}

func MatchAny(*http.Request) bool {
	return true
}

func REST(method, p string) Matcher {
	return func(req *http.Request) bool {
		if !strings.EqualFold(req.Method, method) {
			return false
		}
		if req.URL.Path != "/"+p {
			return false
		}
		return true
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

func readBody(req *http.Request) ([]byte, error) {
	bodyCopy := &bytes.Buffer{}
	r := io.TeeReader(req.Body, bodyCopy)
	req.Body = ioutil.NopCloser(bodyCopy)
	return ioutil.ReadAll(r)
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

func StatusStringResponse(status int, body string) Responder {
	return func(req *http.Request) (*http.Response, error) {
		return httpResponse(status, req, bytes.NewBufferString(body)), nil
	}
}

func JSONResponse(body interface{}) Responder {
	return func(req *http.Request) (*http.Response, error) {
		b, _ := json.Marshal(body)
		return httpResponse(200, req, bytes.NewBuffer(b)), nil
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

func ScopesResponder(scopes string) func(*http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Request:    req,
			Header: map[string][]string{
				"X-Oauth-Scopes": {scopes},
			},
			Body: ioutil.NopCloser(bytes.NewBufferString("")),
		}, nil
	}
}

func httpResponse(status int, req *http.Request, body io.Reader) *http.Response {
	return &http.Response{
		StatusCode: status,
		Request:    req,
		Body:       ioutil.NopCloser(body),
	}
}
