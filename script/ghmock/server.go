package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

const maxRequestBody = 10 << 20

// mockServer answers gh's requests for one run of one binary. Each run gets a
// fresh server, so every run starts from each request's first response. A nil
// checker turns schema checks off.
type mockServer struct {
	state    *state
	checker  *checker
	listener net.Listener
	srv      *http.Server

	mu       sync.Mutex
	cursors  map[string]int
	requests []loggedRequest
}

func startServer(st *state, chk *checker) (*mockServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	m := &mockServer{state: st, checker: chk, listener: ln, cursors: map[string]int{}}
	m.srv = &http.Server{Handler: m, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = m.srv.Serve(ln) }()
	return m, nil
}

// check records what the schema check found for a request and its response.
func (m *mockServer) check(entry *loggedRequest, resp *response) {
	if m.checker == nil {
		return
	}
	res := m.checker.checkRequest(*entry, resp)
	entry.QueryProblems = res.queryProblems
	entry.SchemaProblems = res.responseProblems
	entry.SchemaNote = res.note
	entry.OffSchema = resp != nil && resp.OffSchema
}

// proxyURL is the value gh gets for HTTP_PROXY and HTTPS_PROXY.
func (m *mockServer) proxyURL() string {
	return "http://" + m.listener.Addr().String()
}

// close stops the server and returns the requests it saw, in order.
func (m *mockServer) close() []loggedRequest {
	_ = m.srv.Close()
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]loggedRequest(nil), m.requests...)
}

// discard forgets the requests and response positions recorded so far, such as
// anything gh did while printing its version.
func (m *mockServer) discard() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = nil
	m.cursors = map[string]int{}
}

func (m *mockServer) record(e loggedRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.Seq = len(m.requests) + 1
	m.requests = append(m.requests, e)
}

func (m *mockServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// HTTPS goes through CONNECT tunnels, which would reach a real host.
	if r.Method == http.MethodConnect {
		m.record(loggedRequest{Method: r.Method, Host: r.Host, Status: http.StatusForbidden, Blocked: true})
		http.Error(w, "ghmock: HTTPS tunnels are blocked", http.StatusForbidden)
		return
	}

	host := r.URL.Host
	if host == "" {
		host = r.Host
	}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	entry := loggedRequest{Method: r.Method, Host: hostname, Path: r.URL.Path, RawQuery: r.URL.RawQuery}
	if !isMockHost(hostname) {
		entry.Status = http.StatusForbidden
		entry.Blocked = true
		m.record(entry)
		http.Error(w, "ghmock: only "+mockHost+" is served", http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil {
		entry.Status = http.StatusBadRequest
		m.record(entry)
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "ghmock: reading request body: " + err.Error()})
		return
	}
	if len(body) > maxRequestBody {
		entry.Status = http.StatusRequestEntityTooLarge
		m.record(entry)
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"message": "ghmock: request body exceeds 10 MiB"})
		return
	}
	name, gql := requestName(r.Method, hostname, r.URL.Path, body)
	entry.Name = name
	entry.GraphQL = gql
	if gql == nil && len(body) > 0 {
		entry.Body = string(body)
	}

	responses, err := m.state.loadResponses()
	if err != nil {
		entry.Status = http.StatusInternalServerError
		m.record(entry)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "ghmock: " + err.Error()})
		return
	}

	list := responses[name]
	if len(list) == 0 {
		status, payload := missingResponse(name, gql != nil)
		entry.Status = status
		m.check(&entry, nil)
		m.record(entry)
		writeJSON(w, status, payload)
		return
	}

	m.mu.Lock()
	idx := m.cursors[name]
	m.cursors[name] = idx + 1
	m.mu.Unlock()
	if idx >= len(list) {
		idx = len(list) - 1
	}
	resp := list[idx]

	entry.Status = resp.status()
	entry.Matched = true
	entry.Response = idx + 1
	entry.Responses = len(list)
	m.check(&entry, &resp)
	m.record(entry)

	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	payload := resp.Body
	// responses.json is indented for people to read; gh gets compact JSON.
	var compact bytes.Buffer
	if err := json.Compact(&compact, payload); err == nil {
		payload = compact.Bytes()
	}
	if len(payload) > 0 && w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.WriteHeader(resp.status())
	if len(payload) > 0 && r.Method != http.MethodHead {
		_, _ = w.Write(payload)
	}
}

// missingResponse is what gh gets for a request with no stored response. Its
// message names the request so the caller knows what to add.
func missingResponse(name string, isGraphQL bool) (int, any) {
	msg := "ghmock: no response for " + name
	if isGraphQL {
		return http.StatusOK, map[string]any{"errors": []map[string]string{{"message": msg}}}
	}
	return http.StatusNotFound, map[string]string{"message": msg}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
