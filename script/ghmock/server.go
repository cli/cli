package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/vektah/gqlparser/v2/ast"
)

const maxRequestBody = 10 << 20

// mockServer answers gh's requests for one run of one binary. Each run gets a
// fresh server, so every run starts from each request's first response. A nil
// checker turns schema checks off. GraphQL responses are trimmed to the fields
// gh asked for, except off-schema ones, which are sent as stored.
//
// With a certificate authority, the server answers HTTPS for any host gh opens
// a tunnel to, so gh can use github.com. Without one, as with --localhost, it
// serves github.localhost over plain HTTP and blocks everything else.
type mockServer struct {
	state    *state
	checker  *checker
	ca       *certAuthority
	listener net.Listener
	srv      *http.Server

	// tunnels and tunnelSrv answer the requests inside CONNECT tunnels.
	tunnels    *tunnelListener
	tunnelSrv  *http.Server
	handshakes sync.WaitGroup

	mu       sync.Mutex
	closing  bool
	cursors  map[string]int
	requests []loggedRequest
}

func startServer(st *state, chk *checker, ca *certAuthority) (*mockServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	m := &mockServer{state: st, checker: chk, ca: ca, listener: ln, cursors: map[string]int{}}
	m.srv = &http.Server{Handler: m, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = m.srv.Serve(ln) }()
	if ca != nil {
		m.tunnels = newTunnelListener(ln.Addr())
		m.tunnelSrv = &http.Server{Handler: m, ReadHeaderTimeout: 10 * time.Second}
		go func() { _ = m.tunnelSrv.Serve(m.tunnels) }()
	}
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

// graphQLSchema returns GitHub's GraphQL schema, or nil if checks are off or
// it couldn't be loaded.
func (m *mockServer) graphQLSchema() *ast.Schema {
	if m.checker == nil {
		return nil
	}
	schema, _ := m.checker.graphQL()
	return schema
}

// proxyURL is the value gh gets for HTTP_PROXY and HTTPS_PROXY.
func (m *mockServer) proxyURL() string {
	return "http://" + m.listener.Addr().String()
}

// close stops the server and returns the requests it saw, in order. It waits
// for TLS handshakes still under way, so a rejected certificate is logged.
func (m *mockServer) close() []loggedRequest {
	_ = m.srv.Close()
	m.mu.Lock()
	m.closing = true
	m.mu.Unlock()
	m.handshakes.Wait()
	if m.tunnelSrv != nil {
		_ = m.tunnelSrv.Close()
		_ = m.tunnels.Close()
	}
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
	if r.Method == http.MethodConnect {
		if m.ca != nil {
			m.tunnel(w, r)
			return
		}
		// Without ghmock's authority, a tunnel would reach a real host.
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
	if m.ca == nil && !isMockHost(hostname) {
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
		if responseHasBody(r.Method, status) {
			entry.ResponseBody, _ = json.Marshal(payload)
		}
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

	payload := resp.Body
	if gql != nil && !resp.OffSchema && resp.status() == http.StatusOK {
		payload = trimGraphQL(payload, gql, m.graphQLSchema())
	}
	// responses.json is indented for people to read; gh gets compact JSON.
	var compact bytes.Buffer
	if err := json.Compact(&compact, payload); err == nil {
		payload = compact.Bytes()
	}

	entry.Status = resp.status()
	entry.Matched = true
	entry.Response = idx + 1
	entry.Responses = len(list)
	entry.ResponseHeaders = resp.Headers
	if responseHasBody(r.Method, resp.status()) && json.Valid(payload) {
		entry.ResponseBody = payload
	}
	m.check(&entry, &resp)
	m.record(entry)

	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	if len(payload) > 0 && w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.WriteHeader(resp.status())
	if len(payload) > 0 && responseHasBody(r.Method, resp.status()) {
		_, _ = w.Write(payload)
	}
}

func responseHasBody(method string, status int) bool {
	return method != http.MethodHead && status >= 200 &&
		status != http.StatusNoContent && status != http.StatusNotModified
}

// tunnel answers a CONNECT as the host gh asked for: it completes the TLS
// handshake with a certificate from ghmock's authority, then hands the
// connection to the server that answers the requests inside it.
func (m *mockServer) tunnel(w http.ResponseWriter, r *http.Request) {
	hostname := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		hostname = h
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "ghmock: can't open a tunnel", http.StatusInternalServerError)
		return
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	if !m.startHandshake() {
		_ = conn.Close()
		return
	}
	defer m.handshakes.Done()
	if _, err := rw.WriteString("HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
		_ = conn.Close()
		return
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return
	}

	tlsConn := tls.Server(&bufferedConn{Conn: conn, r: rw.Reader}, m.ca.serverConfig(hostname))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = tlsConn.Close()
		// A client that hangs up before the handshake hasn't rejected anything.
		if !errors.Is(err, io.EOF) {
			m.record(loggedRequest{Method: http.MethodConnect, Host: r.Host, TLSError: err.Error()})
		}
		return
	}
	m.tunnels.push(tlsConn)
}

// startHandshake counts a TLS handshake that close waits for. It returns false
// once the server is closing.
func (m *mockServer) startHandshake() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing {
		return false
	}
	m.handshakes.Add(1)
	return true
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
