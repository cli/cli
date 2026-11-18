package catrust

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTransport(t *testing.T) {
	originalKeys := distrustedCAKeys
	t.Cleanup(func() { distrustedCAKeys = originalKeys })
	root := issueCertificate(t, "test root", nil, nil, true)
	otherRoot := issueCertificate(t, "other root", nil, nil, true)
	intermediate := issueCertificate(t, "intermediate", &root, nil, true)
	crossSigned := issueCertificate(t, "intermediate", &otherRoot, intermediate.key, true)
	leaf := issueCertificate(t, "test server", &intermediate, nil, false)
	rootHash := fmt.Sprintf("%x", sha256.Sum256(root.cert.RawSubjectPublicKeyInfo))
	otherHash := fmt.Sprintf("%x", sha256.Sum256(otherRoot.cert.RawSubjectPublicKeyInfo))
	intermediateHash := fmt.Sprintf("%x", sha256.Sum256(intermediate.cert.RawSubjectPublicKeyInfo))

	tests := []struct {
		name        string
		presented   []testCertificate
		roots       []testCertificate
		blocked     string
		serverName  string
		previousErr error
		wantErr     string
		wantChains  int
	}{
		{
			name:       "unaffected chain",
			presented:  []testCertificate{leaf, intermediate},
			roots:      []testCertificate{root},
			blocked:    otherHash,
			wantChains: 1,
		},
		{
			name:      "root not sent by the peer is blocked",
			presented: []testCertificate{leaf, intermediate},
			roots:     []testCertificate{root},
			blocked:   rootHash,
			wantErr:   "distrusted CA",
		},
		{
			name:      "same CA key cross-signed under a different trusted root is blocked",
			presented: []testCertificate{leaf, crossSigned},
			roots:     []testCertificate{otherRoot},
			blocked:   intermediateHash,
			wantErr:   "distrusted CA",
		},
		{
			name:       "clean first chain is sufficient",
			presented:  []testCertificate{leaf, intermediate, crossSigned},
			roots:      []testCertificate{root, otherRoot},
			blocked:    otherHash,
			wantChains: 1,
		},
		{
			name:       "clean last chain is sufficient",
			presented:  []testCertificate{leaf, intermediate, crossSigned},
			roots:      []testCertificate{root, otherRoot},
			blocked:    rootHash,
			wantChains: 1,
		},
		{
			name:      "no clean chain remains",
			presented: []testCertificate{leaf, intermediate, crossSigned},
			roots:     []testCertificate{root, otherRoot},
			blocked:   intermediateHash,
			wantErr:   "distrusted CA",
		},
		{
			name:       "unused blocked peer certificate does not cause rejection",
			presented:  []testCertificate{leaf, intermediate, otherRoot},
			roots:      []testCertificate{root},
			blocked:    otherHash,
			wantChains: 1,
		},
		{
			name:      "unknown authority still fails standard verification",
			presented: []testCertificate{leaf, intermediate},
			roots:     []testCertificate{otherRoot},
			blocked:   rootHash,
			wantErr:   "certificate signed by unknown authority",
		},
		{
			name:       "hostname mismatch still fails standard verification",
			presented:  []testCertificate{leaf, intermediate},
			roots:      []testCertificate{root},
			blocked:    rootHash,
			serverName: "wrong.example",
			wantErr:    "certificate is not valid for any names",
		},
		{
			name:        "existing verifier can still reject the connection",
			presented:   []testCertificate{leaf, intermediate},
			roots:       []testCertificate{root},
			blocked:     otherHash,
			previousErr: errors.New("existing verifier rejected connection"),
			wantErr:     "existing verifier rejected connection",
			wantChains:  1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			distrustedCAKeys = map[string]bool{tt.blocked: true}
			server := certificateServer(t, tt.presented, 0)
			roots := x509.NewCertPool()
			for _, root := range tt.roots {
				roots.AddCert(root.cert)
			}
			var chains [][]*x509.Certificate
			base := &http.Transport{
				DisableKeepAlives: true,
				TLSClientConfig: &tls.Config{
					RootCAs:    roots,
					ServerName: tt.serverName,
					VerifyConnection: func(state tls.ConnectionState) error {
						chains = state.VerifiedChains
						return tt.previousErr
					},
				},
			}
			t.Cleanup(base.CloseIdleConnections)
			transport, err := NewTransport(base)
			require.NoError(t, err)

			client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			res, err := client.Get(server.URL)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				if tt.previousErr != nil {
					require.ErrorIs(t, err, tt.previousErr)
				}
			} else {
				require.NoError(t, err)
				require.NoError(t, res.Body.Close())
				assert.Equal(t, http.StatusOK, res.StatusCode)
				if len(tt.roots) == 2 {
					require.Len(t, res.TLS.VerifiedChains, 2)
					for _, chain := range res.TLS.VerifiedChains {
						require.NotEmpty(t, chain, "filter must not modify TLS-owned chains")
					}
				}
			}
			require.Len(t, chains, tt.wantChains)
			for _, chain := range chains {
				for _, cert := range chain {
					assert.False(t, distrustedCAKeys[fmt.Sprintf("%x", sha256.Sum256(cert.RawSubjectPublicKeyInfo))],
						"existing verifier must see only allowed chains")
				}
			}

			if tt.wantErr == "distrusted CA" {
				// The policy clone must not change the original transport or TLS config.
				originalClient := &http.Client{Transport: base, Timeout: 5 * time.Second}
				res, err := originalClient.Get(server.URL)
				require.NoError(t, err)
				require.NoError(t, res.Body.Close())
			}
		})
	}

	t.Run("peer verifier cannot approve only a distrusted chain", func(t *testing.T) {
		distrustedCAKeys = map[string]bool{rootHash: true}
		server := certificateServer(t, []testCertificate{leaf, intermediate, crossSigned}, 0)
		roots := x509.NewCertPool()
		roots.AddCert(root.cert)
		roots.AddCert(otherRoot.cert)
		base := &http.Transport{
			DisableKeepAlives: true,
			TLSClientConfig: &tls.Config{
				RootCAs: roots,
				VerifyPeerCertificate: func(_ [][]byte, chains [][]*x509.Certificate) error {
					for _, chain := range chains {
						if chain[len(chain)-1].Equal(root.cert) {
							return nil
						}
					}
					return errors.New("peer verifier requires the distrusted root")
				},
			},
		}
		t.Cleanup(base.CloseIdleConnections)
		original := &http.Client{Transport: base, Timeout: 5 * time.Second}
		res, err := original.Get(server.URL)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.Len(t, res.TLS.VerifiedChains, 2)

		_, err = NewTransport(base)
		require.ErrorContains(t, err, "does not support VerifyPeerCertificate callbacks")
	})

	t.Run("policy does not replace host roots", func(t *testing.T) {
		transport, err := NewTransport(&http.Transport{})
		require.NoError(t, err)
		configured, ok := transport.(*http.Transport)
		require.True(t, ok)
		assert.Nil(t, configured.TLSClientConfig.RootCAs)
		require.ErrorContains(t, configured.TLSClientConfig.VerifyConnection(tls.ConnectionState{}),
			"distrusted CA")
	})
}

func TestNewTransportResumption(t *testing.T) {
	originalKeys := distrustedCAKeys
	t.Cleanup(func() { distrustedCAKeys = originalKeys })
	root := issueCertificate(t, "test root", nil, nil, true)
	leaf := issueCertificate(t, "test server", &root, nil, false)
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		t.Run(tls.VersionName(version), func(t *testing.T) {
			server := certificateServer(t, []testCertificate{leaf}, version)
			roots := x509.NewCertPool()
			roots.AddCert(root.cert)
			base := &http.Transport{
				DisableKeepAlives: true,
				TLSClientConfig: &tls.Config{
					RootCAs:            roots,
					ClientSessionCache: tls.NewLRUClientSessionCache(1),
				},
			}
			t.Cleanup(base.CloseIdleConnections)
			client := &http.Client{Transport: base, Timeout: 5 * time.Second}
			res, err := client.Get(server.URL)
			require.NoError(t, err)
			_, err = io.Copy(io.Discard, res.Body)
			require.NoError(t, err)
			require.NoError(t, res.Body.Close())
			require.False(t, res.TLS.DidResume)

			// Share the session cache, but add a policy after the first connection.
			// The resumed connection must be rejected using its cached verified chain.
			distrustedCAKeys = map[string]bool{fmt.Sprintf("%x", sha256.Sum256(root.cert.RawSubjectPublicKeyInfo)): true}
			transport, err := NewTransport(base)
			require.NoError(t, err)
			configured, ok := transport.(*http.Transport)
			require.True(t, ok)
			verify := configured.TLSClientConfig.VerifyConnection
			var resumed bool
			configured.TLSClientConfig.VerifyConnection = func(state tls.ConnectionState) error {
				resumed = state.DidResume
				return verify(state)
			}
			blockedClient := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			_, err = blockedClient.Get(server.URL)
			require.ErrorContains(t, err, "distrusted CA")
			assert.True(t, resumed)
		})
	}
}

type testCertificate struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

func issueCertificate(t *testing.T, name string, parent *testCertificate, key *ecdsa.PrivateKey, ca bool) testCertificate {
	t.Helper()
	var err error
	if key == nil {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  ca,
	}
	if ca {
		template.KeyUsage |= x509.KeyUsageCertSign
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	if parent == nil {
		parent = &testCertificate{cert: template, key: key}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent.cert, &key.PublicKey, parent.key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return testCertificate{cert: cert, key: key}
}

func certificateServer(t *testing.T, chain []testCertificate, version uint16) *httptest.Server {
	t.Helper()
	var certificates [][]byte
	for _, cert := range chain {
		certificates = append(certificates, cert.cert.Raw)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{{Certificate: certificates, PrivateKey: chain[0].key}},
		MinVersion:   version,
		MaxVersion:   version,
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}
