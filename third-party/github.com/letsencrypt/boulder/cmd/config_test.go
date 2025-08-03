package cmd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"
)

func TestDBConfigURL(t *testing.T) {
	tests := []struct {
		conf     DBConfig
		expected string
	}{
		{
			// Test with one config file that has no trailing newline
			conf:     DBConfig{DBConnectFile: "testdata/test_dburl"},
			expected: "test@tcp(testhost:3306)/testDB?readTimeout=800ms&writeTimeout=800ms",
		},
		{
			// Test with a config file that *has* a trailing newline
			conf:     DBConfig{DBConnectFile: "testdata/test_dburl_newline"},
			expected: "test@tcp(testhost:3306)/testDB?readTimeout=800ms&writeTimeout=800ms",
		},
	}

	for _, tc := range tests {
		url, err := tc.conf.URL()
		test.AssertNotError(t, err, "Failed calling URL() on DBConfig")
		test.AssertEquals(t, url, tc.expected)
	}
}

func TestPasswordConfig(t *testing.T) {
	tests := []struct {
		pc       PasswordConfig
		expected string
	}{
		{pc: PasswordConfig{}, expected: ""},
		{pc: PasswordConfig{PasswordFile: "testdata/test_secret"}, expected: "secret"},
	}

	for _, tc := range tests {
		password, err := tc.pc.Pass()
		test.AssertNotError(t, err, "Failed to retrieve password")
		test.AssertEquals(t, password, tc.expected)
	}
}

func TestTLSConfigLoad(t *testing.T) {
	null := "/dev/null"
	nonExistent := "[nonexistent]"
	tmp := t.TempDir()
	cert := path.Join(tmp, "TestTLSConfigLoad.cert.pem")
	key := path.Join(tmp, "TestTLSConfigLoad.key.pem")
	caCert := path.Join(tmp, "TestTLSConfigLoad.cacert.pem")

	rootKey, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	test.AssertNotError(t, err, "creating test root key")
	rootTemplate := &x509.Certificate{
		Subject:      pkix.Name{CommonName: "test root"},
		SerialNumber: big.NewInt(12345),
		NotBefore:    time.Now().Add(-24 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		IsCA:         true,
	}
	rootCert, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, rootKey.Public(), rootKey)
	test.AssertNotError(t, err, "creating test root cert")
	err = os.WriteFile(caCert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootCert}), os.ModeAppend)
	test.AssertNotError(t, err, "writing test root cert to disk")

	intKey, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	test.AssertNotError(t, err, "creating test intermediate key")
	intKeyBytes, err := x509.MarshalECPrivateKey(intKey)
	test.AssertNotError(t, err, "marshalling test intermediate key")
	err = os.WriteFile(key, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: intKeyBytes}), os.ModeAppend)
	test.AssertNotError(t, err, "writing test intermediate key cert to disk")

	intTemplate := &x509.Certificate{
		Subject:      pkix.Name{CommonName: "test intermediate"},
		SerialNumber: big.NewInt(67890),
		NotBefore:    time.Now().Add(-12 * time.Hour),
		NotAfter:     time.Now().Add(12 * time.Hour),
		IsCA:         true,
	}
	intCert, err := x509.CreateCertificate(rand.Reader, intTemplate, rootTemplate, intKey.Public(), rootKey)
	test.AssertNotError(t, err, "creating test intermediate cert")
	err = os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: intCert}), os.ModeAppend)
	test.AssertNotError(t, err, "writing test intermediate cert to disk")

	testCases := []struct {
		TLSConfig
		want string
	}{
		{TLSConfig{"", null, null}, "nil CertFile in TLSConfig"},
		{TLSConfig{null, "", null}, "nil KeyFile in TLSConfig"},
		{TLSConfig{null, null, ""}, "nil CACertFile in TLSConfig"},
		{TLSConfig{nonExistent, key, caCert}, "loading key pair.*no such file or directory"},
		{TLSConfig{cert, nonExistent, caCert}, "loading key pair.*no such file or directory"},
		{TLSConfig{cert, key, nonExistent}, "reading CA cert from.*no such file or directory"},
		{TLSConfig{null, key, caCert}, "loading key pair.*failed to find any PEM data"},
		{TLSConfig{cert, null, caCert}, "loading key pair.*failed to find any PEM data"},
		{TLSConfig{cert, key, null}, "parsing CA certs"},
		{TLSConfig{cert, key, caCert}, ""},
	}
	for _, tc := range testCases {
		title := [3]string{tc.CertFile, tc.KeyFile, tc.CACertFile}
		for i := range title {
			if title[i] == "" {
				title[i] = "nil"
			}
		}
		t.Run(strings.Join(title[:], "_"), func(t *testing.T) {
			_, err := tc.TLSConfig.Load(metrics.NoopRegisterer)
			if err == nil && tc.want == "" {
				return
			}
			if err == nil {
				t.Errorf("got no error")
			}
			if matched, _ := regexp.MatchString(tc.want, err.Error()); !matched {
				t.Errorf("got error %q, wanted %q", err, tc.want)
			}
		})
	}
}

func TestHMACKeyConfigLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		expectedErr bool
	}{
		{
			name:        "Valid key",
			content:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedErr: false,
		},
		{
			name:        "Empty file",
			content:     "",
			expectedErr: true,
		},
		{
			name:        "Just under 256-bit",
			content:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",
			expectedErr: true,
		},
		{
			name:        "Just over 256-bit",
			content:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef01",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tempKeyFile, err := os.CreateTemp("", "*")
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			defer os.Remove(tempKeyFile.Name())

			_, err = tempKeyFile.WriteString(tt.content)
			if err != nil {
				t.Fatalf("failed to write to temp file: %v", err)
			}
			tempKeyFile.Close()

			hmacKeyConfig := HMACKeyConfig{KeyFile: tempKeyFile.Name()}
			_, err = hmacKeyConfig.Load()
			if (err != nil) != tt.expectedErr {
				t.Errorf("expected error: %v, got: %v", tt.expectedErr, err)
			}
		})
	}
}
