package creds

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmhodges/clock"

	"github.com/letsencrypt/boulder/test"
)

func TestServerTransportCredentials(t *testing.T) {
	_, badCert := test.ThrowAwayCert(t, clock.New())
	goodCert := &x509.Certificate{
		DNSNames:    []string{"creds-test"},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	acceptedSANs := map[string]struct{}{
		"creds-test": {},
	}
	servTLSConfig := &tls.Config{}

	// NewServerCredentials with a nil serverTLSConfig should return an error
	_, err := NewServerCredentials(nil, acceptedSANs)
	test.AssertEquals(t, err, ErrNilServerConfig)

	// A creds with a nil acceptedSANs list should consider any peer valid
	wrappedCreds, err := NewServerCredentials(servTLSConfig, nil)
	test.AssertNotError(t, err, "NewServerCredentials failed with nil acceptedSANs")
	bcreds := wrappedCreds.(*serverTransportCredentials)
	err = bcreds.validateClient(tls.ConnectionState{})
	test.AssertNotError(t, err, "validateClient() errored for emptyState")

	// A creds with a empty acceptedSANs list should consider any peer valid
	wrappedCreds, err = NewServerCredentials(servTLSConfig, map[string]struct{}{})
	test.AssertNotError(t, err, "NewServerCredentials failed with empty acceptedSANs")
	bcreds = wrappedCreds.(*serverTransportCredentials)
	err = bcreds.validateClient(tls.ConnectionState{})
	test.AssertNotError(t, err, "validateClient() errored for emptyState")

	// A properly-initialized creds should fail to verify an empty ConnectionState
	bcreds = &serverTransportCredentials{servTLSConfig, acceptedSANs}
	err = bcreds.validateClient(tls.ConnectionState{})
	test.AssertEquals(t, err, ErrEmptyPeerCerts)

	// A creds should reject peers that don't have a leaf certificate with
	// a SAN on the accepted list.
	err = bcreds.validateClient(tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{badCert},
	})
	var errSANNotAccepted ErrSANNotAccepted
	test.AssertErrorWraps(t, err, &errSANNotAccepted)

	// A creds should accept peers that have a leaf certificate with a SAN
	// that is on the accepted list
	err = bcreds.validateClient(tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{goodCert},
	})
	test.AssertNotError(t, err, "validateClient(rightState) failed")

	// A creds configured with an IP SAN in the accepted list should accept a peer
	// that has a leaf certificate containing an IP address SAN present in the
	// accepted list.
	acceptedIPSans := map[string]struct{}{
		"127.0.0.1": {},
	}
	bcreds = &serverTransportCredentials{servTLSConfig, acceptedIPSans}
	err = bcreds.validateClient(tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{goodCert},
	})
	test.AssertNotError(t, err, "validateClient(rightState) failed with an IP accepted SAN list")
}

func TestClientTransportCredentials(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to generate test key")

	temp := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		DNSNames:              []string{"A"},
		NotBefore:             time.Unix(1000, 0),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	derA, err := x509.CreateCertificate(rand.Reader, temp, temp, priv.Public(), priv)
	test.AssertNotError(t, err, "x509.CreateCertificate failed")
	certA, err := x509.ParseCertificate(derA)
	test.AssertNotError(t, err, "x509.ParserCertificate failed")
	temp.DNSNames[0] = "B"
	derB, err := x509.CreateCertificate(rand.Reader, temp, temp, priv.Public(), priv)
	test.AssertNotError(t, err, "x509.CreateCertificate failed")
	certB, err := x509.ParseCertificate(derB)
	test.AssertNotError(t, err, "x509.ParserCertificate failed")
	roots := x509.NewCertPool()
	roots.AddCert(certA)
	roots.AddCert(certB)

	serverA := httptest.NewUnstartedServer(nil)
	serverA.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{derA}, PrivateKey: priv}}}
	serverB := httptest.NewUnstartedServer(nil)
	serverB.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{derB}, PrivateKey: priv}}}

	tc := NewClientCredentials(roots, []tls.Certificate{}, "")

	serverA.StartTLS()
	defer serverA.Close()
	addrA := serverA.Listener.Addr().String()
	rawConnA, err := net.Dial("tcp", addrA)
	test.AssertNotError(t, err, "net.Dial failed")
	defer func() {
		_ = rawConnA.Close()
	}()

	conn, _, err := tc.ClientHandshake(context.Background(), "A:2020", rawConnA)
	test.AssertNotError(t, err, "tc.ClientHandshake failed")
	test.Assert(t, conn != nil, "tc.ClientHandshake returned a nil net.Conn")

	serverB.StartTLS()
	defer serverB.Close()
	addrB := serverB.Listener.Addr().String()
	rawConnB, err := net.Dial("tcp", addrB)
	test.AssertNotError(t, err, "net.Dial failed")
	defer func() {
		_ = rawConnB.Close()
	}()

	conn, _, err = tc.ClientHandshake(context.Background(), "B:3030", rawConnB)
	test.AssertNotError(t, err, "tc.ClientHandshake failed")
	test.Assert(t, conn != nil, "tc.ClientHandshake returned a nil net.Conn")

	// Test timeout
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	test.AssertNotError(t, err, "net.Listen failed")
	defer func() {
		_ = ln.Close()
	}()
	addrC := ln.Addr().String()
	stop := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = ln.Accept()
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()

	rawConnC, err := net.Dial("tcp", addrC)
	test.AssertNotError(t, err, "net.Dial failed")
	defer func() {
		_ = rawConnB.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	conn, _, err = tc.ClientHandshake(ctx, "A:2020", rawConnC)
	test.AssertError(t, err, "tc.ClientHandshake didn't timeout")
	test.AssertEquals(t, err.Error(), "context deadline exceeded")
	test.Assert(t, conn == nil, "tc.ClientHandshake returned a non-nil net.Conn on failure")

	stop <- struct{}{}
}

type brokenConn struct{}

func (bc *brokenConn) Read([]byte) (int, error) {
	return 0, &net.OpError{}
}

func (bc *brokenConn) Write([]byte) (int, error) {
	return 0, &net.OpError{}
}

func (bc *brokenConn) LocalAddr() net.Addr              { return nil }
func (bc *brokenConn) RemoteAddr() net.Addr             { return nil }
func (bc *brokenConn) Close() error                     { return nil }
func (bc *brokenConn) SetDeadline(time.Time) error      { return nil }
func (bc *brokenConn) SetReadDeadline(time.Time) error  { return nil }
func (bc *brokenConn) SetWriteDeadline(time.Time) error { return nil }

func TestClientReset(t *testing.T) {
	tc := NewClientCredentials(nil, []tls.Certificate{}, "")
	_, _, err := tc.ClientHandshake(context.Background(), "T:1010", &brokenConn{})
	test.AssertError(t, err, "ClientHandshake succeeded with brokenConn")
	var netErr net.Error
	test.AssertErrorWraps(t, err, &netErr)
}
