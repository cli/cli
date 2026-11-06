package main

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"sync"
	"time"
)

// certAuthority is a throwaway certificate authority for one ghmock run. gh
// trusts it through SSL_CERT_FILE, so ghmock can answer gh's HTTPS requests to
// github.com, and any other host, itself.
type certAuthority struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	certPEM []byte

	mu     sync.Mutex
	leaves map[string]*tls.Certificate
}

func newCertAuthority() (*certAuthority, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "ghmock throwaway certificate authority"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &certAuthority{
		cert:    cert,
		key:     key,
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		leaves:  map[string]*tls.Certificate{},
	}, nil
}

// randomSerial returns a random serial number, so certificates from different
// runs never share one.
func randomSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
}

// leaf returns a certificate for host signed by the authority, making it the
// first time it's asked for.
func (ca *certAuthority) leaf(host string) (*tls.Certificate, error) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	ca.mu.Lock()
	defer ca.mu.Unlock()
	if c, ok := ca.leaves[host]; ok {
		return c, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    ca.cert.NotBefore,
		NotAfter:     ca.cert.NotAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, err
	}
	c := &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	ca.leaves[host] = c
	return c, nil
}

// serverConfig presents a certificate for the host gh names in its TLS
// handshake, or for fallback when it names none.
func (ca *certAuthority) serverConfig(fallback string) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		// The server inside the tunnel speaks HTTP/1.1 only.
		NextProtos: []string{"http/1.1"},
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			host := hello.ServerName
			if host == "" {
				host = fallback
			}
			return ca.leaf(host)
		},
	}
}

// bufferedConn reads first whatever the CONNECT request's reader buffered,
// which can include the start of the TLS handshake.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// tunnelListener hands the TLS connections from CONNECT tunnels to the
// http.Server that answers the requests inside them.
type tunnelListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
	addr  net.Addr
}

func newTunnelListener(addr net.Addr) *tunnelListener {
	return &tunnelListener{conns: make(chan net.Conn), done: make(chan struct{}), addr: addr}
}

func (l *tunnelListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *tunnelListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *tunnelListener) Addr() net.Addr { return l.addr }

// push hands c to the server, or closes it once the listener is closed.
func (l *tunnelListener) push(c net.Conn) {
	select {
	case l.conns <- c:
	case <-l.done:
		_ = c.Close()
	}
}
