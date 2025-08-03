// Package acme provides ACME client functionality tailored to the needs of the
// load-generator. It is not a general purpose ACME client library.
package acme

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	// NewNonceEndpoint is the directory key for the newNonce endpoint.
	NewNonceEndpoint Endpoint = "newNonce"
	// NewAccountEndpoint is the directory key for the newAccount endpoint.
	NewAccountEndpoint Endpoint = "newAccount"
	// NewOrderEndpoint is the directory key for the newOrder endpoint.
	NewOrderEndpoint Endpoint = "newOrder"
	// RevokeCertEndpoint is the directory key for the revokeCert endpoint.
	RevokeCertEndpoint Endpoint = "revokeCert"
	// KeyChangeEndpoint is the directory key for the keyChange endpoint.
	KeyChangeEndpoint Endpoint = "keyChange"
)

var (
	// ErrEmptyDirectory is returned if NewDirectory is provided and empty directory URL.
	ErrEmptyDirectory = errors.New("directoryURL must not be empty")
	// ErrInvalidDirectoryURL is returned if NewDirectory is provided an invalid directory URL.
	ErrInvalidDirectoryURL = errors.New("directoryURL is not a valid URL")
	// ErrInvalidDirectoryHTTPCode is returned if NewDirectory is provided a directory URL
	// that returns something other than HTTP Status OK to a GET request.
	ErrInvalidDirectoryHTTPCode = errors.New("GET request to directoryURL did not result in HTTP Status 200")
	// ErrInvalidDirectoryJSON is returned if NewDirectory is provided a directory URL
	// that returns invalid JSON.
	ErrInvalidDirectoryJSON = errors.New("GET request to directoryURL returned invalid JSON")
	// ErrInvalidDirectoryMeta is returned if NewDirectory is provided a directory
	// URL that returns a directory resource with an invalid or  missing "meta" key.
	ErrInvalidDirectoryMeta = errors.New(`server's directory resource had invalid or missing "meta" key`)
	// ErrInvalidTermsOfService is returned if NewDirectory is provided
	// a directory URL that returns a directory resource with an invalid or
	// missing "termsOfService" key in the "meta" map.
	ErrInvalidTermsOfService = errors.New(`server's directory resource had invalid or missing "meta.termsOfService" key`)

	// RequiredEndpoints is a slice of Endpoint keys that must be present in the
	// ACME server's directory. The load-generator uses each of these endpoints
	// and expects to be able to find a URL for each in the server's directory
	// resource.
	RequiredEndpoints = []Endpoint{
		NewNonceEndpoint, NewAccountEndpoint,
		NewOrderEndpoint, RevokeCertEndpoint,
	}
)

// Endpoint represents a string key used for looking up an endpoint URL in an ACME
// server directory resource.
//
// E.g. NewOrderEndpoint -> "newOrder" -> "https://acme.example.com/acme/v1/new-order-plz"
//
// See "ACME Resource Types" registry - RFC 8555 Section 9.7.5.
type Endpoint string

// ErrMissingEndpoint is an error returned if NewDirectory is provided an ACME
// server directory URL that is missing a key for a required endpoint in the
// response JSON. See also RequiredEndpoints.
type ErrMissingEndpoint struct {
	endpoint Endpoint
}

// Error returns the error message for an ErrMissingEndpoint error.
func (e ErrMissingEndpoint) Error() string {
	return fmt.Sprintf(
		"directoryURL JSON was missing required key for %q endpoint",
		e.endpoint,
	)
}

// ErrInvalidEndpointURL is an error returned if NewDirectory is provided an
// ACME server directory URL that has an invalid URL for a required endpoint.
// See also RequiredEndpoints.
type ErrInvalidEndpointURL struct {
	endpoint Endpoint
	value    string
}

// Error returns the error message for an ErrInvalidEndpointURL error.
func (e ErrInvalidEndpointURL) Error() string {
	return fmt.Sprintf(
		"directoryURL JSON had invalid URL value (%q) for %q endpoint",
		e.value, e.endpoint)
}

// Directory is a type for holding URLs extracted from the ACME server's
// Directory resource.
//
// See RFC 8555 Section 7.1.1 "Directory".
//
// Its public API is read-only and therefore it is safe for concurrent access.
type Directory struct {
	// TermsOfService is the URL identifying the current terms of service found in
	// the ACME server's directory resource's "meta" field.
	TermsOfService string
	// endpointURLs is a map from endpoint name to URL.
	endpointURLs map[Endpoint]string
}

// getRawDirectory validates the provided directoryURL and makes a GET request
// to fetch the raw bytes of the server's directory resource. If the URL is
// invalid, if there is an error getting the directory bytes, or if the HTTP
// response code is not 200 an error is returned.
func getRawDirectory(directoryURL string) ([]byte, error) {
	if directoryURL == "" {
		return nil, ErrEmptyDirectory
	}

	if _, err := url.Parse(directoryURL); err != nil {
		return nil, ErrInvalidDirectoryURL
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
			TLSClientConfig: &tls.Config{
				// Bypassing CDN or testing against Pebble instances can cause
				// validation failures. For a **test-only** tool its acceptable to skip
				// cert verification of the ACME server's HTTPs certificate.
				InsecureSkipVerify: true,
			},
			MaxIdleConns:    1,
			IdleConnTimeout: 15 * time.Second,
		},
		Timeout: 10 * time.Second,
	}

	resp, err := httpClient.Get(directoryURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrInvalidDirectoryHTTPCode
	}

	rawDirectory, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return rawDirectory, nil
}

// termsOfService reads the termsOfService key from the meta key of the raw
// directory resource.
func termsOfService(rawDirectory map[string]interface{}) (string, error) {
	var directoryMeta map[string]interface{}

	if rawDirectoryMeta, ok := rawDirectory["meta"]; !ok {
		return "", ErrInvalidDirectoryMeta
	} else if directoryMetaMap, ok := rawDirectoryMeta.(map[string]interface{}); !ok {
		return "", ErrInvalidDirectoryMeta
	} else {
		directoryMeta = directoryMetaMap
	}

	rawToSURL, ok := directoryMeta["termsOfService"]
	if !ok {
		return "", ErrInvalidTermsOfService
	}

	tosURL, ok := rawToSURL.(string)
	if !ok {
		return "", ErrInvalidTermsOfService
	}
	return tosURL, nil
}

// NewDirectory creates a Directory populated from the ACME directory resource
// returned by a GET request to the provided directoryURL. It also checks that
// the fetched directory contains each of the RequiredEndpoints.
func NewDirectory(directoryURL string) (*Directory, error) {
	// Fetch the raw directory JSON
	dirContents, err := getRawDirectory(directoryURL)
	if err != nil {
		return nil, err
	}

	// Unmarshal the directory
	var dirResource map[string]interface{}
	err = json.Unmarshal(dirContents, &dirResource)
	if err != nil {
		return nil, ErrInvalidDirectoryJSON
	}

	// serverURL tries to find a valid url.URL for the provided endpoint in
	// the unmarshaled directory resource.
	serverURL := func(name Endpoint) (*url.URL, error) {
		if rawURL, ok := dirResource[string(name)]; !ok {
			return nil, ErrMissingEndpoint{endpoint: name}
		} else if urlString, ok := rawURL.(string); !ok {
			return nil, ErrInvalidEndpointURL{endpoint: name, value: urlString}
		} else if url, err := url.Parse(urlString); err != nil {
			return nil, ErrInvalidEndpointURL{endpoint: name, value: urlString}
		} else {
			return url, nil
		}
	}

	// Create an empty directory to populate
	directory := &Directory{
		endpointURLs: make(map[Endpoint]string),
	}

	// Every required endpoint must have a valid URL populated from the directory
	for _, endpointName := range RequiredEndpoints {
		url, err := serverURL(endpointName)
		if err != nil {
			return nil, err
		}
		directory.endpointURLs[endpointName] = url.String()
	}

	// Populate the terms-of-service
	tos, err := termsOfService(dirResource)
	if err != nil {
		return nil, err
	}
	directory.TermsOfService = tos
	return directory, nil
}

// EndpointURL returns the string representation of the ACME server's URL for
// the provided endpoint. If the Endpoint is not known an empty string is
// returned.
func (d *Directory) EndpointURL(ep Endpoint) string {
	if url, ok := d.endpointURLs[ep]; ok {
		return url
	}

	return ""
}
