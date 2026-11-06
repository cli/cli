package main

import (
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCertAuthority(t *testing.T) {
	ca, err := newCertAuthority()
	require.NoError(t, err)

	block, _ := pem.Decode(ca.certPEM)
	require.NotNil(t, block, "the authority is written as PEM for SSL_CERT_FILE")
	root, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.True(t, root.IsCA)
	roots := x509.NewCertPool()
	roots.AddCert(root)

	tests := []struct {
		name    string
		host    string
		verify  string
		wantErr bool
	}{
		{name: "the API host", host: "api.github.com", verify: "api.github.com"},
		{name: "another host", host: "uploads.github.com", verify: "uploads.github.com"},
		{name: "host names are case-insensitive", host: "API.GitHub.com", verify: "api.github.com"},
		{name: "an IP address", host: "127.0.0.1", verify: "127.0.0.1"},
		{name: "a certificate is only for its host", host: "api.github.com", verify: "github.com", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := ca.leaf(tt.host)
			require.NoError(t, err)
			leaf, err := x509.ParseCertificate(c.Certificate[0])
			require.NoError(t, err)

			_, err = leaf.Verify(x509.VerifyOptions{DNSName: tt.verify, Roots: roots})
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			again, err := ca.leaf(tt.host)
			require.NoError(t, err)
			assert.Same(t, c, again, "each host's certificate is made once")
		})
	}
}
