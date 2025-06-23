package acme

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/letsencrypt/boulder/test"
)

// Path constants for test cases and mockDirectoryServer handlers.
const (
	wrongStatusCodePath         = "/dir-wrong-status"
	invalidJSONPath             = "/dir-bad-json"
	missingEndpointPath         = "/dir-missing-endpoint"
	invalidEndpointURLPath      = "/dir-invalid-endpoint"
	validDirectoryPath          = "/dir-valid"
	invalidMetaDirectoryPath    = "/dir-valid-meta-invalid"
	invalidMetaDirectoryToSPath = "/dir-valid-meta-valid-tos-invalid"
)

// mockDirectoryServer is an httptest.Server that returns mock data for ACME
// directory GET requests based on the requested path.
type mockDirectoryServer struct {
	*httptest.Server
}

// newMockDirectoryServer creates a mockDirectoryServer that returns mock data
// based on the requested path. The returned server will not be started
// automatically.
func newMockDirectoryServer() *mockDirectoryServer {
	m := http.NewServeMux()

	m.HandleFunc(wrongStatusCodePath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnavailableForLegalReasons)
	})

	m.HandleFunc(invalidJSONPath, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{`)
	})

	m.HandleFunc(missingEndpointPath, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	})

	m.HandleFunc(invalidEndpointURLPath, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			 "newAccount": "",
			 "newNonce": "ht\ntp://bad-scheme",
			 "newOrder": "",
			 "revokeCert": ""
		}`)
	})

	m.HandleFunc(invalidMetaDirectoryPath, func(w http.ResponseWriter, r *http.Request) {
		noMetaDir := `{
			 "keyChange": "https://localhost:14000/rollover-account-key",
			 "newAccount": "https://localhost:14000/sign-me-up",
			 "newNonce": "https://localhost:14000/nonce-plz",
			 "newOrder": "https://localhost:14000/order-plz",
			 "revokeCert": "https://localhost:14000/revoke-cert"
		}`
		fmt.Fprint(w, noMetaDir)
	})

	m.HandleFunc(invalidMetaDirectoryToSPath, func(w http.ResponseWriter, r *http.Request) {
		noToSDir := `{
			 "keyChange": "https://localhost:14000/rollover-account-key",
			 "meta": {
				 "chaos": "reigns"
			 },
			 "newAccount": "https://localhost:14000/sign-me-up",
			 "newNonce": "https://localhost:14000/nonce-plz",
			 "newOrder": "https://localhost:14000/order-plz",
			 "revokeCert": "https://localhost:14000/revoke-cert"
		}`
		fmt.Fprint(w, noToSDir)
	})

	m.HandleFunc(validDirectoryPath, func(w http.ResponseWriter, r *http.Request) {
		validDir := `{
			 "keyChange": "https://localhost:14000/rollover-account-key",
			 "meta": {
					"termsOfService": "data:text/plain,Do%20what%20thou%20wilt"
			 },
			 "newAccount": "https://localhost:14000/sign-me-up",
			 "newNonce": "https://localhost:14000/nonce-plz",
			 "newOrder": "https://localhost:14000/order-plz",
			 "revokeCert": "https://localhost:14000/revoke-cert"
		}`
		fmt.Fprint(w, validDir)
	})

	srv := &mockDirectoryServer{
		Server: httptest.NewUnstartedServer(m),
	}

	return srv
}

// TestNew tests that creating a new Client and populating the endpoint map
// works correctly.
func TestNew(t *testing.T) {
	srv := newMockDirectoryServer()
	srv.Start()
	defer srv.Close()

	srvUrl, _ := url.Parse(srv.URL)
	_, port, _ := net.SplitHostPort(srvUrl.Host)

	testURL := func(path string) string {
		return fmt.Sprintf("http://localhost:%s%s", port, path)
	}

	testCases := []struct {
		Name          string
		DirectoryURL  string
		ExpectedError string
	}{
		{
			Name:          "empty directory URL",
			ExpectedError: ErrEmptyDirectory.Error(),
		},
		{
			Name:          "invalid directory URL",
			DirectoryURL:  "http://" + string([]byte{0x1, 0x7F}),
			ExpectedError: ErrInvalidDirectoryURL.Error(),
		},
		{
			Name:          "unreachable directory URL",
			DirectoryURL:  "http://localhost:1987",
			ExpectedError: "connect: connection refused",
		},
		{
			Name:          "wrong directory HTTP status code",
			DirectoryURL:  testURL(wrongStatusCodePath),
			ExpectedError: ErrInvalidDirectoryHTTPCode.Error(),
		},
		{
			Name:          "invalid directory JSON",
			DirectoryURL:  testURL(invalidJSONPath),
			ExpectedError: ErrInvalidDirectoryJSON.Error(),
		},
		{
			Name:          "directory JSON missing required endpoint",
			DirectoryURL:  testURL(missingEndpointPath),
			ExpectedError: ErrMissingEndpoint{endpoint: NewNonceEndpoint}.Error(),
		},
		{
			Name:         "directory JSON with invalid endpoint URL",
			DirectoryURL: testURL(invalidEndpointURLPath),
			ExpectedError: ErrInvalidEndpointURL{
				endpoint: NewNonceEndpoint,
				value:    "ht\ntp://bad-scheme",
			}.Error(),
		},
		{
			Name:          "directory JSON missing meta key",
			DirectoryURL:  testURL(invalidMetaDirectoryPath),
			ExpectedError: ErrInvalidDirectoryMeta.Error(),
		},
		{
			Name:          "directory JSON missing meta TermsOfService key",
			DirectoryURL:  testURL(invalidMetaDirectoryToSPath),
			ExpectedError: ErrInvalidTermsOfService.Error(),
		},
		{
			Name:         "valid directory",
			DirectoryURL: testURL(validDirectoryPath),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			_, err := NewDirectory(tc.DirectoryURL)
			if err == nil && tc.ExpectedError != "" {
				t.Errorf("expected error %q got nil", tc.ExpectedError)
			} else if err != nil {
				test.AssertContains(t, err.Error(), tc.ExpectedError)
			}
		})
	}
}
