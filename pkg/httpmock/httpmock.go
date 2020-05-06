package httpmock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

type Registry struct {
	mu    sync.Mutex
	stubs []*Stub
}

func (r *Registry) Register(m Matcher, resp Responder) {
	r.stubs = append(r.stubs, &Stub{
		Matcher:   m,
		Responder: resp,
	})
}

type Testing interface {
	Errorf(string, ...interface{})
}

func (r *Registry) Verify(t Testing) {
	n := 0
	for _, s := range r.stubs {
		if !s.matched {
			n++
		}
	}
	if n > 0 {
		t.Errorf("%d unmatched HTTP stubs", n)
	}
}

// RoundTrip satisfies http.RoundTripper
func (r *Registry) RoundTrip(req *http.Request) (*http.Response, error) {
	var stub *Stub

	r.mu.Lock()
	for _, s := range r.stubs {
		if s.matched || !s.Matcher(req) {
			continue
		}
		if stub != nil {
			r.mu.Unlock()
			return nil, fmt.Errorf("more than 1 stub matched %v", req)
		}
		stub = s
	}
	if stub != nil {
		stub.matched = true
	}
	r.mu.Unlock()

	if stub == nil {
		return nil, fmt.Errorf("no registered stubs matched %v", req)
	}
	return stub.Responder(req)
}

type Matcher func(req *http.Request) bool
type Responder func(req *http.Request) (*http.Response, error)

type Stub struct {
	matched   bool
	Matcher   Matcher
	Responder Responder
}

func GraphQL(q string) Matcher {
	re := regexp.MustCompile(q)

	return func(req *http.Request) bool {
		if !strings.EqualFold(req.Method, "POST") {
			return false
		}
		if req.URL.Path != "/graphql" {
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
	return func(*http.Request) (*http.Response, error) {
		return httpResponse(200, bytes.NewBufferString(body)), nil
	}
}

func JSONResponse(body interface{}) Responder {
	return func(*http.Request) (*http.Response, error) {
		b, _ := json.Marshal(body)
		return httpResponse(200, bytes.NewBuffer(b)), nil
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

		return httpResponse(200, bytes.NewBufferString(body)), nil
	}
}

func httpResponse(status int, body io.Reader) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       ioutil.NopCloser(body),
	}
}
