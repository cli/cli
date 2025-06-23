package main

import (
	"crypto"
	"testing"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/test"
)

func TestKeyBlocking(t *testing.T) {
	testCases := []struct {
		name     string
		certPath string
		jwkPath  string
		expected string
	}{
		// NOTE(@cpu): The JWKs and certificates were generated with the same
		// keypair within an algorithm/parameter family. E.g. the RSA JWK public key
		// matches the RSA certificate public key. The ECDSA JWK public key matches
		// the ECDSA certificate public key.
		{
			name:     "P-256 ECDSA JWK",
			jwkPath:  "test/test.ecdsa.jwk.json",
			expected: "cuwGhNNI6nfob5aqY90e7BleU6l7rfxku4X3UTJ3Z7M=",
		},
		{
			name:     "2048 RSA JWK",
			jwkPath:  "test/test.rsa.jwk.json",
			expected: "Qebc1V3SkX3izkYRGNJilm9Bcuvf0oox4U2Rn+b4JOE=",
		},
		{
			name:     "P-256 ECDSA Certificate",
			certPath: "test/test.ecdsa.cert.pem",
			expected: "cuwGhNNI6nfob5aqY90e7BleU6l7rfxku4X3UTJ3Z7M=",
		},
		{
			name:     "2048 RSA Certificate",
			certPath: "test/test.rsa.cert.pem",
			expected: "Qebc1V3SkX3izkYRGNJilm9Bcuvf0oox4U2Rn+b4JOE=",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var key crypto.PublicKey
			var err error
			if tc.jwkPath != "" {
				key, err = keyFromJWK(tc.jwkPath)
			} else {
				key, err = keyFromCert(tc.certPath)
			}
			test.AssertNotError(t, err, "error getting key from input file")
			spkiHash, err := core.KeyDigestB64(key)
			test.AssertNotError(t, err, "error computing spki hash")
			test.AssertEquals(t, spkiHash, tc.expected)
		})
	}
}
