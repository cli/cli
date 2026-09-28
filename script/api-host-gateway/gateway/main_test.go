package main

import (
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayForwardsRewritesAndRecords(t *testing.T) {
	var upstreamHostHeader, upstreamAuthHeader, upstreamAcceptEncoding string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHostHeader = r.Host
		upstreamAuthHeader = r.Header.Get("Authorization")
		upstreamAcceptEncoding = r.Header.Get("Accept-Encoding")

		w.Header().Set("Link", `<https://api.github.com/repositories/1/labels?page=2>; rel="next"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"url":"https://api.github.com/repos/cli/cli"}`)
	}))
	defer upstream.Close()

	logPath := filepath.Join(t.TempDir(), "gateway.jsonl")
	rec, err := newRecorder(logPath)
	require.NoError(t, err)
	defer rec.Close()

	proxy := newProxy(
		"gh-gateway.internal",
		&url.URL{Scheme: "http", Host: "api.github.com"},
		strings.TrimPrefix(upstream.URL, "http://"),
		rec,
	)

	gateway := httptest.NewServer(recordingHandler(proxy, rec))
	defer gateway.Close()

	req, err := http.NewRequest(http.MethodGet, gateway.URL+"/repos/cli/cli", nil)
	require.NoError(t, err)
	req.Host = "gh-gateway.internal"
	req.Header.Set("Authorization", "token secret")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	t.Run("forwards to the upstream as the canonical host", func(t *testing.T) {
		assert.Equal(t, "api.github.com", upstreamHostHeader)
		assert.Equal(t, "token secret", upstreamAuthHeader)
		assert.Equal(t, "identity", upstreamAcceptEncoding)
	})

	t.Run("rewrites the canonical host out of headers and body", func(t *testing.T) {
		assert.Equal(t, `<https://gh-gateway.internal/repositories/1/labels?page=2>; rel="next"`, resp.Header.Get("Link"))
		assert.Equal(t, `{"url":"https://gh-gateway.internal/repos/cli/cli"}`, string(body))
		assert.Equal(t, strconv.Itoa(len(body)), resp.Header.Get("Content-Length"))
	})

	t.Run("records the request as the client addressed it", func(t *testing.T) {
		entries := readLog(t, logPath)
		require.Len(t, entries, 1)
		assert.Equal(t, http.MethodGet, entries[0].Method)
		assert.Equal(t, "/repos/cli/cli", entries[0].Path)
		assert.Equal(t, "gh-gateway.internal", entries[0].Host)
		assert.True(t, entries[0].AuthHeader)
		assert.Equal(t, http.StatusOK, entries[0].Status)
	})
}

func TestGatewayRecordsUpstreamFailures(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "gateway.jsonl")
	rec, err := newRecorder(logPath)
	require.NoError(t, err)
	defer rec.Close()

	// 127.0.0.1:1 is not listening, standing in for an unreachable upstream.
	proxy := newProxy("gh-gateway.internal", &url.URL{Scheme: "http", Host: "api.github.com"}, "127.0.0.1:1", rec)

	gateway := httptest.NewServer(recordingHandler(proxy, rec))
	defer gateway.Close()

	resp, err := http.Get(gateway.URL + "/user")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)

	entries := readLog(t, logPath)
	require.Len(t, entries, 1)
	assert.Equal(t, "/user", entries[0].Path)
	assert.False(t, entries[0].AuthHeader)
	assert.NotEmpty(t, entries[0].Error)
}

func TestGeneratedServerCertificateChainsToTheCA(t *testing.T) {
	caPEM, serverCert, err := generateCertificates("gh-gateway.internal")
	require.NoError(t, err)

	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(caPEM))

	leaf, err := x509.ParseCertificate(serverCert.Certificate[0])
	require.NoError(t, err)

	_, err = leaf.Verify(x509.VerifyOptions{
		Roots:     roots,
		DNSName:   "gh-gateway.internal",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	require.NoError(t, err)

	_, err = leaf.Verify(x509.VerifyOptions{
		Roots:     roots,
		DNSName:   "api.github.com",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	require.Error(t, err, "the gateway must not be able to impersonate the canonical host")
}

func readLog(t *testing.T, path string) []record {
	t.Helper()

	contents, err := os.ReadFile(path)
	require.NoError(t, err)

	var entries []record
	for line := range strings.SplitSeq(strings.TrimSpace(string(contents)), "\n") {
		if line == "" {
			continue
		}
		var entry record
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		entries = append(entries, entry)
	}
	return entries
}
