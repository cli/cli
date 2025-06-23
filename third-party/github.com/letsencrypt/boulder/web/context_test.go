package web

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/test"
)

type myHandler struct{}

func (m myHandler) ServeHTTP(e *RequestEvent, w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(201)
	e.Endpoint = "/endpoint"
	_, _ = w.Write([]byte("hi"))
}

func TestLogCode(t *testing.T) {
	mockLog := blog.UseMock()
	th := NewTopHandler(mockLog, myHandler{})
	req, err := http.NewRequest("GET", "/thisisignored", &bytes.Reader{})
	if err != nil {
		t.Fatal(err)
	}
	th.ServeHTTP(httptest.NewRecorder(), req)
	expected := `INFO: GET /endpoint 0 201 0 0.0.0.0 JSON={}`
	if len(mockLog.GetAllMatching(expected)) != 1 {
		t.Errorf("Expected exactly one log line matching %q. Got \n%s",
			expected, strings.Join(mockLog.GetAllMatching(".*"), "\n"))
	}
}

type codeHandler struct{}

func (ch codeHandler) ServeHTTP(e *RequestEvent, w http.ResponseWriter, r *http.Request) {
	e.Endpoint = "/endpoint"
	_, _ = w.Write([]byte("hi"))
}

func TestStatusCodeLogging(t *testing.T) {
	mockLog := blog.UseMock()
	th := NewTopHandler(mockLog, codeHandler{})
	req, err := http.NewRequest("GET", "/thisisignored", &bytes.Reader{})
	if err != nil {
		t.Fatal(err)
	}
	th.ServeHTTP(httptest.NewRecorder(), req)
	expected := `INFO: GET /endpoint 0 200 0 0.0.0.0 JSON={}`
	if len(mockLog.GetAllMatching(expected)) != 1 {
		t.Errorf("Expected exactly one log line matching %q. Got \n%s",
			expected, strings.Join(mockLog.GetAllMatching(".*"), "\n"))
	}
}

func TestOrigin(t *testing.T) {
	mockLog := blog.UseMock()
	th := NewTopHandler(mockLog, myHandler{})
	req, err := http.NewRequest("GET", "/thisisignored", &bytes.Reader{})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("Origin", "https://example.com")
	th.ServeHTTP(httptest.NewRecorder(), req)
	expected := `INFO: GET /endpoint 0 201 0 0.0.0.0 JSON={.*"Origin":"https://example.com"}`
	if len(mockLog.GetAllMatching(expected)) != 1 {
		t.Errorf("Expected exactly one log line matching %q. Got \n%s",
			expected, strings.Join(mockLog.GetAllMatching(".*"), "\n"))
	}
}

type hostHeaderHandler struct {
	f func(*RequestEvent, http.ResponseWriter, *http.Request)
}

func (hhh hostHeaderHandler) ServeHTTP(e *RequestEvent, w http.ResponseWriter, r *http.Request) {
	hhh.f(e, w, r)
}

func TestHostHeaderRewrite(t *testing.T) {
	mockLog := blog.UseMock()
	hhh := hostHeaderHandler{f: func(_ *RequestEvent, _ http.ResponseWriter, r *http.Request) {
		t.Helper()
		test.AssertEquals(t, r.Host, "localhost")
	}}
	th := NewTopHandler(mockLog, &hhh)

	req, err := http.NewRequest("GET", "/", &bytes.Reader{})
	test.AssertNotError(t, err, "http.NewRequest failed")
	req.Host = "localhost:80"
	fmt.Println("here")
	th.ServeHTTP(httptest.NewRecorder(), req)

	req, err = http.NewRequest("GET", "/", &bytes.Reader{})
	test.AssertNotError(t, err, "http.NewRequest failed")
	req.Host = "localhost:443"
	req.TLS = &tls.ConnectionState{}
	th.ServeHTTP(httptest.NewRecorder(), req)

	req, err = http.NewRequest("GET", "/", &bytes.Reader{})
	test.AssertNotError(t, err, "http.NewRequest failed")
	req.Host = "localhost:443"
	req.TLS = nil
	th.ServeHTTP(httptest.NewRecorder(), req)

	hhh.f = func(_ *RequestEvent, _ http.ResponseWriter, r *http.Request) {
		t.Helper()
		test.AssertEquals(t, r.Host, "localhost:123")
	}
	req, err = http.NewRequest("GET", "/", &bytes.Reader{})
	test.AssertNotError(t, err, "http.NewRequest failed")
	req.Host = "localhost:123"
	th.ServeHTTP(httptest.NewRecorder(), req)
}
