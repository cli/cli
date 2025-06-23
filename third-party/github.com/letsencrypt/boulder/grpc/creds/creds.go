package creds

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"

	"google.golang.org/grpc/credentials"
)

var (
	ErrClientHandshakeNop = errors.New(
		"boulder/grpc/creds: Client-side handshakes are not implemented with " +
			"serverTransportCredentials")
	ErrServerHandshakeNop = errors.New(
		"boulder/grpc/creds: Server-side handshakes are not implemented with " +
			"clientTransportCredentials")
	ErrOverrideServerNameNop = errors.New(
		"boulder/grpc/creds: OverrideServerName() is not implemented")
	ErrNilServerConfig = errors.New(
		"boulder/grpc/creds: `serverConfig` must not be nil")
	ErrEmptyPeerCerts = errors.New(
		"boulder/grpc/creds: validateClient given state with empty PeerCertificates")
)

type ErrSANNotAccepted struct {
	got, expected []string
}

func (e ErrSANNotAccepted) Error() string {
	return fmt.Sprintf("boulder/grpc/creds: client certificate SAN was invalid. "+
		"Got %q, expected one of %q.", e.got, e.expected)
}

// clientTransportCredentials is a grpc/credentials.TransportCredentials which supports
// connecting to, and verifying multiple DNS names
type clientTransportCredentials struct {
	roots   *x509.CertPool
	clients []tls.Certificate
	// If set, this is used as the hostname to validate on certificates, instead
	// of the value passed to ClientHandshake by grpc.
	hostOverride string
}

// NewClientCredentials returns a new initialized grpc/credentials.TransportCredentials for client usage
func NewClientCredentials(rootCAs *x509.CertPool, clientCerts []tls.Certificate, hostOverride string) credentials.TransportCredentials {
	return &clientTransportCredentials{rootCAs, clientCerts, hostOverride}
}

// ClientHandshake does the authentication handshake specified by the corresponding
// authentication protocol on rawConn for clients. It returns the authenticated
// connection and the corresponding auth information about the connection.
// Implementations must use the provided context to implement timely cancellation.
func (tc *clientTransportCredentials) ClientHandshake(ctx context.Context, addr string, rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	var err error
	host := tc.hostOverride
	if host == "" {
		// IMPORTANT: Don't wrap the errors returned from this method. gRPC expects to be
		// able to check err.Temporary to spot temporary errors and reconnect when they happen.
		host, _, err = net.SplitHostPort(addr)
		if err != nil {
			return nil, nil, err
		}
	}
	conn := tls.Client(rawConn, &tls.Config{
		ServerName:   host,
		RootCAs:      tc.roots,
		Certificates: tc.clients,
	})
	err = conn.HandshakeContext(ctx)
	if err != nil {
		_ = rawConn.Close()
		return nil, nil, err
	}
	return conn, nil, nil
}

// ServerHandshake is not implemented for a `clientTransportCredentials`, use
// a `serverTransportCredentials` if you require `ServerHandshake`.
func (tc *clientTransportCredentials) ServerHandshake(rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return nil, nil, ErrServerHandshakeNop
}

// Info returns information about the transport protocol used
func (tc *clientTransportCredentials) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "tls"}
}

// GetRequestMetadata returns nil, nil since TLS credentials do not have metadata.
func (tc *clientTransportCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return nil, nil
}

// RequireTransportSecurity always returns true because TLS is transport security
func (tc *clientTransportCredentials) RequireTransportSecurity() bool {
	return true
}

// Clone returns a copy of the clientTransportCredentials
func (tc *clientTransportCredentials) Clone() credentials.TransportCredentials {
	return NewClientCredentials(tc.roots, tc.clients, tc.hostOverride)
}

// OverrideServerName is not implemented and here only to satisfy the interface
func (tc *clientTransportCredentials) OverrideServerName(serverNameOverride string) error {
	return ErrOverrideServerNameNop
}

// serverTransportCredentials is a grpc/credentials.TransportCredentials which supports
// filtering acceptable peer connections by a list of accepted client certificate SANs
type serverTransportCredentials struct {
	serverConfig *tls.Config
	acceptedSANs map[string]struct{}
}

// NewServerCredentials returns a new initialized grpc/credentials.TransportCredentials for server usage
func NewServerCredentials(serverConfig *tls.Config, acceptedSANs map[string]struct{}) (credentials.TransportCredentials, error) {
	if serverConfig == nil {
		return nil, ErrNilServerConfig
	}

	return &serverTransportCredentials{serverConfig, acceptedSANs}, nil
}

// validateClient checks a peer's client certificate's SAN entries against
// a list of accepted SANs. If the client certificate does not have a SAN on the
// list it is rejected.
//
// Note 1: This function *only* verifies the SAN entries! Callers are expected to
// have provided the `tls.ConnectionState` returned from a validate (e.g.
// non-error producing) `conn.Handshake()`.
//
// Note 2: We do *not* consider the client certificate subject common name. The
// CN field is deprecated and should be present as a DNS SAN!
func (tc *serverTransportCredentials) validateClient(peerState tls.ConnectionState) error {
	/*
	 * If there's no list of accepted SANs, all clients are OK
	 *
	 * TODO(@cpu): This should be converted to a hard error at initialization time
	 * once we have deployed & updated all gRPC configurations to have an accepted
	 * SAN list configured
	 */
	if len(tc.acceptedSANs) == 0 {
		return nil
	}

	// If `conn.Handshake()` is called before `validateClient` this should not
	// occur. We return an error in this event primarily for unit tests that may
	// call `validateClient` with manufactured & artificial connection states.
	if len(peerState.PeerCertificates) < 1 {
		return ErrEmptyPeerCerts
	}

	// Since we call `conn.Handshake()` before `validateClient` and ensure
	// a non-error response we don't need to validate anything except the presence
	// of an acceptable SAN in the leaf entry of `PeerCertificates`. The tls
	// package's `serverHandshake` and in particular, `processCertsFromClient`
	// will address everything else as an error returned from `Handshake()`.
	leaf := peerState.PeerCertificates[0]

	// Combine both the DNS and IP address subjectAlternativeNames into a single
	// list for checking.
	var receivedSANs []string
	receivedSANs = append(receivedSANs, leaf.DNSNames...)
	for _, ip := range leaf.IPAddresses {
		receivedSANs = append(receivedSANs, ip.String())
	}

	for _, name := range receivedSANs {
		if _, ok := tc.acceptedSANs[name]; ok {
			return nil
		}
	}

	// If none of the DNS or IP SANs on the leaf certificate matched the
	// acceptable list, the client isn't valid and we error
	var acceptableSANs []string
	for k := range tc.acceptedSANs {
		acceptableSANs = append(acceptableSANs, k)
	}
	return ErrSANNotAccepted{receivedSANs, acceptableSANs}
}

// ServerHandshake does the authentication handshake for servers. It returns
// the authenticated connection and the corresponding auth information about
// the connection.
func (tc *serverTransportCredentials) ServerHandshake(rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	// Perform the server <- client TLS handshake. This will validate the peer's
	// client certificate.
	conn := tls.Server(rawConn, tc.serverConfig)
	err := conn.Handshake()
	if err != nil {
		return nil, nil, err
	}

	// In addition to the validation from `conn.Handshake()` we apply further
	// constraints on what constitutes a valid peer
	err = tc.validateClient(conn.ConnectionState())
	if err != nil {
		return nil, nil, err
	}

	return conn, credentials.TLSInfo{State: conn.ConnectionState()}, nil
}

// ClientHandshake is not implemented for a `serverTransportCredentials`, use
// a `clientTransportCredentials` if you require `ClientHandshake`.
func (tc *serverTransportCredentials) ClientHandshake(ctx context.Context, addr string, rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return nil, nil, ErrClientHandshakeNop
}

// Info provides the ProtocolInfo of this TransportCredentials.
func (tc *serverTransportCredentials) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "tls"}
}

// GetRequestMetadata returns nil, nil since TLS credentials do not have metadata.
func (tc *serverTransportCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return nil, nil
}

// RequireTransportSecurity always returns true because TLS is transport security
func (tc *serverTransportCredentials) RequireTransportSecurity() bool {
	return true
}

// Clone returns a copy of the serverTransportCredentials
func (tc *serverTransportCredentials) Clone() credentials.TransportCredentials {
	clone, _ := NewServerCredentials(tc.serverConfig, tc.acceptedSANs)
	return clone
}

// OverrideServerName is not implemented and here only to satisfy the interface
func (tc *serverTransportCredentials) OverrideServerName(serverNameOverride string) error {
	return ErrOverrideServerNameNop
}
