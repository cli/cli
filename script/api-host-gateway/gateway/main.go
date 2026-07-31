// Command gateway is a recording TLS reverse proxy that stands in for a
// corporate API gateway during the api_host black box test.
//
// It terminates TLS with a certificate it generates for itself, forwards every
// request to the real GitHub API, and appends a JSONL record for each request
// it handled. Requests are forwarded to a pinned upstream address so the
// gateway keeps working after the test blackholes api.github.com in
// /etc/hosts.
//
// Responses are buffered and every occurrence of the upstream host is rewritten
// to the gateway host, in headers such as Link and in JSON bodies. Real
// gateways have to do this because GitHub does not rewrite the absolute URLs it
// returns, so without it a paginated request would send the client straight
// back to the canonical host.
package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type recordKey struct{}

// record is a single line of the gateway's JSONL request log. Host is the Host
// header as the client sent it, before the gateway rewrites it for the
// upstream, so it is evidence of where the client believed it was connecting.
type record struct {
	Time       string `json:"time"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Host       string `json:"host"`
	AuthHeader bool   `json:"auth_header"`
	Status     int    `json:"status"`
	Error      string `json:"error,omitempty"`
}

func main() {
	listen := flag.String("listen", "127.0.0.2:443", "address to listen on, must be port 443 because api_host cannot carry a port")
	gatewayHost := flag.String("gateway-host", "gh-gateway.internal", "hostname clients use to reach this gateway")
	upstreamHost := flag.String("upstream-host", "api.github.com", "canonical GitHub API host to forward to")
	upstreamAddr := flag.String("upstream-addr", "", "pinned host:port to dial for the upstream, bypassing DNS")
	caOut := flag.String("ca-out", "", "path to write the generated CA certificate to")
	logOut := flag.String("log", "", "path to append JSONL request records to")
	readyOut := flag.String("ready", "", "path to create once the gateway is accepting connections")
	flag.Parse()

	if err := run(*listen, *gatewayHost, *upstreamHost, *upstreamAddr, *caOut, *logOut, *readyOut); err != nil {
		log.Fatalf("gateway: %v", err)
	}
}

func run(listen, gatewayHost, upstreamHost, upstreamAddr, caOut, logOut, readyOut string) error {
	for name, value := range map[string]string{
		"-upstream-addr": upstreamAddr,
		"-ca-out":        caOut,
		"-log":           logOut,
	} {
		if value == "" {
			return fmt.Errorf("%s is required", name)
		}
	}

	rec, err := newRecorder(logOut)
	if err != nil {
		return fmt.Errorf("opening log: %w", err)
	}
	defer rec.Close()

	proxy := newProxy(gatewayHost, &url.URL{Scheme: "https", Host: upstreamHost}, upstreamAddr, rec)

	// Bind before writing the CA out, so a gateway that cannot start does not
	// replace the CA that a running one is serving with.
	tcpListener, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", listen, err)
	}

	caPEM, serverCert, err := generateCertificates(gatewayHost)
	if err != nil {
		return fmt.Errorf("generating certificates: %w", err)
	}
	if err := os.WriteFile(caOut, caPEM, 0o600); err != nil {
		return fmt.Errorf("writing CA certificate: %w", err)
	}

	tlsListener := tls.NewListener(tcpListener, &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		MinVersion:   tls.VersionTLS12,
	})

	if readyOut != "" {
		if err := os.WriteFile(readyOut, []byte(listen), 0o600); err != nil {
			return fmt.Errorf("writing ready file: %w", err)
		}
	}

	log.Printf("gateway listening on %s as %s, forwarding to %s at %s", listen, gatewayHost, upstreamHost, upstreamAddr)

	server := &http.Server{
		Handler:           recordingHandler(proxy, rec),
		ReadHeaderTimeout: 30 * time.Second,
	}
	return server.Serve(tlsListener)
}

// recordingHandler attaches a record to the request context so the proxy can
// fill in the outcome, then writes exactly one line per request once the proxy
// is done with it.
func recordingHandler(proxy http.Handler, rec *recorder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entry := &record{
			Time:       time.Now().UTC().Format(time.RFC3339Nano),
			Method:     r.Method,
			Path:       r.URL.RequestURI(),
			Host:       r.Host,
			AuthHeader: r.Header.Get("Authorization") != "",
		}
		proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), recordKey{}, entry)))
		rec.Write(entry)
	})
}

// newProxy builds the reverse proxy. The upstream URL carries the canonical
// host that requests are forwarded to and that responses are rewritten from,
// while dialAddr is the address actually dialled, so the gateway keeps working
// once the canonical host is blackholed.
func newProxy(gatewayHost string, upstream *url.URL, dialAddr string, rec *recorder) *httputil.ReverseProxy {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, dialAddr)
		},
		TLSClientConfig: &tls.Config{
			ServerName: upstream.Host,
			MinVersion: tls.VersionTLS12,
		},
		ForceAttemptHTTP2: true,
	}

	return &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = upstream.Scheme
			pr.Out.URL.Host = upstream.Host
			pr.Out.Host = upstream.Host
			// The response has to be readable for rewriting, and asking for an
			// identity encoding is cheaper than decompressing it again.
			pr.Out.Header.Set("Accept-Encoding", "identity")
		},
		ModifyResponse: func(resp *http.Response) error {
			if entry, ok := resp.Request.Context().Value(recordKey{}).(*record); ok {
				entry.Status = resp.StatusCode
			}
			return rewriteResponse(resp, upstream.Host, gatewayHost)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if entry, ok := r.Context().Value(recordKey{}).(*record); ok {
				entry.Status = http.StatusBadGateway
				entry.Error = err.Error()
			}
			rec.Logf("upstream error for %s %s: %v", r.Method, r.URL.RequestURI(), err)
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, "gateway: upstream error: %v\n", err)
		},
	}
}

// rewriteResponse replaces the upstream host with the gateway host everywhere
// it appears, so that clients following URLs the API handed them stay on the
// gateway.
func rewriteResponse(resp *http.Response, upstreamHost, gatewayHost string) error {
	for key, values := range resp.Header {
		for i, value := range values {
			if strings.Contains(value, upstreamHost) {
				values[i] = strings.ReplaceAll(value, upstreamHost, gatewayHost)
			}
		}
		resp.Header[key] = values
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading upstream body: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("closing upstream body: %w", err)
	}

	body = bytes.ReplaceAll(body, []byte(upstreamHost), []byte(gatewayHost))

	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	// The upstream was asked for an identity encoding, but drop any stale
	// encoding header rather than describe the rewritten body incorrectly.
	resp.Header.Del("Content-Encoding")

	return nil
}

func generateCertificates(gatewayHost string) ([]byte, tls.Certificate, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, tls.Certificate{}, err
	}

	notBefore := time.Now().Add(-time.Hour)
	notAfter := time.Now().Add(24 * time.Hour)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "gh api_host test CA"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, tls.Certificate{}, err
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, tls.Certificate{}, err
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: gatewayHost},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{gatewayHost},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv4(127, 0, 0, 2)},
	}

	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, tls.Certificate{}, err
	}

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	return caPEM, tls.Certificate{
		Certificate: [][]byte{serverDER, caDER},
		PrivateKey:  serverKey,
	}, nil
}

type recorder struct {
	mu   sync.Mutex
	file *os.File
}

func newRecorder(path string) (*recorder, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &recorder{file: file}, nil
}

func (r *recorder) Write(entry *record) {
	line, err := json.Marshal(entry)
	if err != nil {
		r.Logf("marshalling record: %v", err)
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.file.Write(append(line, '\n')); err != nil {
		log.Printf("gateway: writing record: %v", err)
	}
}

func (r *recorder) Logf(format string, args ...any) {
	log.Printf("gateway: "+format, args...)
}

func (r *recorder) Close() {
	if err := r.file.Close(); err != nil {
		log.Printf("gateway: closing log: %v", err)
	}
}
