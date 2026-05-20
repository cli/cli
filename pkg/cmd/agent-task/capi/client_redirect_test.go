package capi

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCAPITransport_DoesNotLeakBearerOnCrossHostRedirect(t *testing.T) {
	const secretToken = "test-bearer-must-not-leak"

	var (
		mu               sync.Mutex
		attackerAuthSeen string
	)

	attackerLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	attackerSrv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attackerAuthSeen = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})}
	go func() { _ = attackerSrv.Serve(attackerLn) }()
	defer attackerSrv.Close()
	attackerURL := "http://" + attackerLn.Addr().String()

	legitSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", attackerURL+"/leaked")
		w.WriteHeader(http.StatusFound)
	}))
	defer legitSrv.Close()

	httpClient := &http.Client{Transport: http.DefaultTransport}
	httpClient.Transport = newCAPITransport(secretToken, legitSrv.URL, httpClient.Transport)

	req, err := http.NewRequest("GET", legitSrv.URL+"/v1/jobs", nil)
	require.NoError(t, err)
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, attackerAuthSeen,
		"capiTransport must not re-attach the Bearer token on a cross-host redirect; "+
			"got Authorization=%q on the cross-host hop", attackerAuthSeen)
}

func TestCAPITransport_AttachesBearerOnSameHostRedirect(t *testing.T) {
	const secretToken = "test-bearer-allowed-same-host"

	var (
		mu              sync.Mutex
		secondHopAuth   string
		secondHopCalled bool
	)

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", srv.URL+"/v1/jobs/second-hop")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/v1/jobs/second-hop", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		secondHopCalled = true
		secondHopAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	httpClient := &http.Client{Transport: http.DefaultTransport}
	httpClient.Transport = newCAPITransport(secretToken, srv.URL, httpClient.Transport)

	req, err := http.NewRequest("GET", srv.URL+"/v1/jobs", nil)
	require.NoError(t, err)
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	require.True(t, secondHopCalled, "second hop on same host should have been reached")
	assert.Equal(t, "Bearer "+secretToken, secondHopAuth,
		"capiTransport must still attach the Bearer token on a same-host redirect")
}
