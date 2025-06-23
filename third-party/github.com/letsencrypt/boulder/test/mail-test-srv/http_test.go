package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func reqAndRecorder(t testing.TB, method, relativeUrl string, body io.Reader) (*httptest.ResponseRecorder, *http.Request) {
	endURL := fmt.Sprintf("http://localhost:9381%s", relativeUrl)
	r, err := http.NewRequest(method, endURL, body)
	if err != nil {
		t.Fatalf("could not construct request: %v", err)
	}
	return httptest.NewRecorder(), r
}

func TestHTTPClear(t *testing.T) {
	srv := mailSrv{}
	w, r := reqAndRecorder(t, "POST", "/clear", nil)
	srv.allReceivedMail = []rcvdMail{{}}
	srv.httpClear(w, r)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if len(srv.allReceivedMail) != 0 {
		t.Error("/clear failed to clear mail buffer")
	}

	w, r = reqAndRecorder(t, "GET", "/clear", nil)
	srv.allReceivedMail = []rcvdMail{{}}
	srv.httpClear(w, r)
	if w.Code != 405 {
		t.Errorf("expected 405, got %d", w.Code)
	}
	if len(srv.allReceivedMail) != 1 {
		t.Error("GET /clear cleared the mail buffer")
	}
}

func TestHTTPCount(t *testing.T) {
	srv := mailSrv{}
	srv.allReceivedMail = []rcvdMail{
		{From: "a", To: "b"},
		{From: "a", To: "b"},
		{From: "a", To: "c"},
		{From: "c", To: "a"},
		{From: "c", To: "b"},
	}

	tests := []struct {
		URL   string
		Count int
	}{
		{URL: "/count", Count: 5},
		{URL: "/count?to=b", Count: 3},
		{URL: "/count?to=c", Count: 1},
	}

	var buf bytes.Buffer
	for _, test := range tests {
		w, r := reqAndRecorder(t, "GET", test.URL, nil)
		buf.Reset()
		w.Body = &buf

		srv.httpCount(w, r)
		if w.Code != 200 {
			t.Errorf("%s: expected 200, got %d", test.URL, w.Code)
		}
		n, err := strconv.Atoi(strings.TrimSpace(buf.String()))
		if err != nil {
			t.Errorf("%s: expected a number, got '%s'", test.URL, buf.String())
		} else if n != test.Count {
			t.Errorf("%s: expected %d, got %d", test.URL, test.Count, n)
		}
	}
}
