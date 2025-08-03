package challtestsrvclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client is an HTTP client for https://github.com/letsencrypt/challtestsrv's
// management interface (test/chall-test-srv).
type Client struct {
	baseURL string
}

// NewClient creates a new Client using the provided baseURL, or defaults to
// http://10.77.77.77:8055 if none is provided.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://10.77.77.77:8055"
	}
	return &Client{baseURL: baseURL}
}

const (
	setIPv4        = "set-default-ipv4"
	setIPv6        = "set-default-ipv6"
	delHistory     = "clear-request-history"
	getHTTPHistory = "http-request-history"
	getDNSHistory  = "dns-request-history"
	getALPNHistory = "tlsalpn01-request-history"
	addA           = "add-a"
	delA           = "clear-a"
	addAAAA        = "add-aaaa"
	delAAAA        = "clear-aaaa"
	addCAA         = "add-caa"
	delCAA         = "clear-caa"
	addRedirect    = "add-redirect"
	delRedirect    = "del-redirect"
	addHTTP        = "add-http01"
	delHTTP        = "del-http01"
	addTXT         = "set-txt"
	delTXT         = "clear-txt"
	addALPN        = "add-tlsalpn01"
	delALPN        = "del-tlsalpn01"
	addServfail    = "set-servfail"
	delServfail    = "clear-servfail"
)

func (c *Client) postURL(path string, body interface{}) ([]byte, error) {
	endpoint, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, fmt.Errorf("joining URL %q with path %q: %w", c.baseURL, path, err)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshalling payload for %s: %w", endpoint, err)
	}

	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("sending POST to %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, endpoint)
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", endpoint, err)
	}
	return respBytes, nil
}

// SetDefaultIPv4 sets the challenge server's default IPv4 address used to
// respond to A queries when there are no specific mock A addresses for the
// hostname being queried. Provide an empty string as the default address to
// disable answering A queries except for hosts that have mock A addresses
// added. Any failure returns an error that includes both the relevant operation
// and the payload.
func (c *Client) SetDefaultIPv4(addr string) ([]byte, error) {
	payload := map[string]string{"ip": addr}
	resp, err := c.postURL(setIPv4, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while setting default IPv4 to %q (payload: %v): %w",
			addr, payload, err,
		)
	}
	return resp, nil
}

// SetDefaultIPv6 sets the challenge server's default IPv6 address used to
// respond to AAAA queries when there are no specific mock AAAA addresses for
// the hostname being queried. Provide an empty string as the default address to
// disable answering AAAA queries except for hosts that have mock AAAA addresses
// added. Any failure returns an error that includes both the relevant operation
// and the payload.
func (c *Client) SetDefaultIPv6(addr string) ([]byte, error) {
	payload := map[string]string{"ip": addr}
	resp, err := c.postURL(setIPv6, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while setting default IPv6 to %q (payload: %v): %w",
			addr, payload, err,
		)
	}
	return resp, nil
}

// AddARecord adds a mock A response to the challenge server's DNS interface for
// the given host and IPv4 addresses. Any failure returns an error that includes
// both the relevant operation and the payload.
func (c *Client) AddARecord(host string, addresses []string) ([]byte, error) {
	payload := map[string]interface{}{
		"host":      host,
		"addresses": addresses,
	}
	resp, err := c.postURL(addA, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while adding A record for host %q (payload: %v): %w",
			host, payload, err,
		)
	}
	return resp, nil
}

// RemoveARecord removes a mock A response from the challenge server's DNS
// interface for the given host. Any failure returns an error that includes both
// the relevant operation and the payload.
func (c *Client) RemoveARecord(host string) ([]byte, error) {
	payload := map[string]string{"host": host}
	resp, err := c.postURL(delA, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while removing A record for host %q (payload: %v): %w",
			host, payload, err,
		)
	}
	return resp, nil
}

// AddAAAARecord adds a mock AAAA response to the challenge server's DNS
// interface for the given host and IPv6 addresses. Any failure returns an error
// that includes both the relevant operation and the payload.
func (c *Client) AddAAAARecord(host string, addresses []string) ([]byte, error) {
	payload := map[string]interface{}{
		"host":      host,
		"addresses": addresses,
	}
	resp, err := c.postURL(addAAAA, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while adding AAAA record for host %q (payload: %v): %w",
			host, payload, err,
		)
	}
	return resp, nil
}

// RemoveAAAARecord removes mock AAAA response from the challenge server's DNS
// interface for the given host. Any failure returns an error that includes both
// the relevant operation and the payload.
func (c *Client) RemoveAAAARecord(host string) ([]byte, error) {
	payload := map[string]string{"host": host}
	resp, err := c.postURL(delAAAA, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while removing AAAA record for host %q (payload: %v): %w",
			host, payload, err,
		)
	}
	return resp, nil
}

// AddCAAIssue adds a mock CAA response to the challenge server's DNS interface.
// The mock CAA response will contain one policy with an "issue" tag specifying
// the provided value. Any failure returns an error that includes both the
// relevant operation and the payload.
func (c *Client) AddCAAIssue(host, value string) ([]byte, error) {
	payload := map[string]interface{}{
		"host": host,
		"policies": []map[string]string{
			{"tag": "issue", "value": value},
		},
	}
	resp, err := c.postURL(addCAA, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while adding CAA issue for host %q, val %q (payload: %v): %w",
			host, value, payload, err,
		)
	}
	return resp, nil
}

// RemoveCAAIssue removes a mock CAA response from the challenge server's DNS
// interface for the given host. Any failure returns an error that includes both
// the relevant operation and the payload.
func (c *Client) RemoveCAAIssue(host string) ([]byte, error) {
	payload := map[string]string{"host": host}
	resp, err := c.postURL(delCAA, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while removing CAA issue for host %q (payload: %v): %w",
			host, payload, err,
		)
	}
	return resp, nil
}

// HTTPRequest is a single HTTP request in the request history.
type HTTPRequest struct {
	URL        string `json:"URL"`
	Host       string `json:"Host"`
	HTTPS      bool   `json:"HTTPS"`
	ServerName string `json:"ServerName"`
	UserAgent  string `json:"UserAgent"`
}

// HTTPRequestHistory fetches the challenge server's HTTP request history for
// the given host.
func (c *Client) HTTPRequestHistory(host string) ([]HTTPRequest, error) {
	payload := map[string]string{"host": host}
	raw, err := c.postURL(getHTTPHistory, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while fetching HTTP request history for host %q (payload: %v): %w",
			host, payload, err,
		)
	}
	var data []HTTPRequest
	err = json.Unmarshal(raw, &data)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling HTTP request history: %w", err)
	}
	return data, nil
}

func (c *Client) clearRequestHistory(host, typ string) ([]byte, error) {
	return c.postURL(delHistory, map[string]string{"host": host, "type": typ})
}

// ClearHTTPRequestHistory clears the challenge server's HTTP request history
// for the given host. Any failure returns an error that includes both the
// relevant operation and the payload.
func (c *Client) ClearHTTPRequestHistory(host string) ([]byte, error) {
	resp, err := c.clearRequestHistory(host, "http")
	if err != nil {
		return nil, fmt.Errorf(
			"while clearing HTTP request history for host %q: %w", host, err,
		)
	}
	return resp, nil
}

// AddHTTPRedirect adds a redirect to the challenge server's HTTP interfaces for
// HTTP requests to the given path directing the client to the targetURL.
// Redirects are not served for HTTPS requests. Any failure returns an error
// that includes both the relevant operation and the payload.
func (c *Client) AddHTTPRedirect(path, targetURL string) ([]byte, error) {
	payload := map[string]string{"path": path, "targetURL": targetURL}
	resp, err := c.postURL(addRedirect, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while adding HTTP redirect for path %q -> %q (payload: %v): %w",
			path, targetURL, payload, err,
		)
	}
	return resp, nil
}

// RemoveHTTPRedirect removes a redirect from the challenge server's HTTP
// interfaces for the given path. Any failure returns an error that includes
// both the relevant operation and the payload.
func (c *Client) RemoveHTTPRedirect(path string) ([]byte, error) {
	payload := map[string]string{"path": path}
	resp, err := c.postURL(delRedirect, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while removing HTTP redirect for path %q (payload: %v): %w",
			path, payload, err,
		)
	}
	return resp, nil
}

// AddHTTP01Response adds an ACME HTTP-01 challenge response for the provided
// token under the /.well-known/acme-challenge/ path of the challenge test
// server's HTTP interfaces. The given keyauth will be returned as the HTTP
// response body for requests to the challenge token. Any failure returns an
// error that includes both the relevant operation and the payload.
func (c *Client) AddHTTP01Response(token, keyauth string) ([]byte, error) {
	payload := map[string]string{"token": token, "content": keyauth}
	resp, err := c.postURL(addHTTP, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while adding HTTP-01 challenge response for token %q (payload: %v): %w",
			token, payload, err,
		)
	}
	return resp, nil
}

// RemoveHTTP01Response removes an ACME HTTP-01 challenge response for the
// provided token from the challenge test server. Any failure returns an error
// that includes both the relevant operation and the payload.
func (c *Client) RemoveHTTP01Response(token string) ([]byte, error) {
	payload := map[string]string{"token": token}
	resp, err := c.postURL(delHTTP, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while removing HTTP-01 challenge response for token %q (payload: %v): %w",
			token, payload, err,
		)
	}
	return resp, nil
}

// AddServfailResponse configures the challenge test server to return SERVFAIL
// for all queries made for the provided host. This will override any other
// mocks for the host until removed with remove_servfail_response. Any failure
// returns an error that includes both the relevant operation and the payload.
func (c *Client) AddServfailResponse(host string) ([]byte, error) {
	payload := map[string]string{"host": host}
	resp, err := c.postURL(addServfail, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while adding SERVFAIL response for host %q (payload: %v): %w",
			host, payload, err,
		)
	}
	return resp, nil
}

// RemoveServfailResponse undoes the work of AddServfailResponse, removing the
// SERVFAIL configuration for the given host. Any failure returns an error that
// includes both the relevant operation and the payload.
func (c *Client) RemoveServfailResponse(host string) ([]byte, error) {
	payload := map[string]string{"host": host}
	resp, err := c.postURL(delServfail, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while removing SERVFAIL response for host %q (payload: %v): %w",
			host, payload, err,
		)
	}
	return resp, nil
}

// AddDNS01Response adds an ACME DNS-01 challenge response for the provided host
// to the challenge test server's DNS interfaces. The value is hashed and
// base64-encoded using RawURLEncoding, and served for TXT queries to
// _acme-challenge.<host>. Any failure returns an error that includes both the
// relevant operation and the payload.
func (c *Client) AddDNS01Response(host, value string) ([]byte, error) {
	host = "_acme-challenge." + host
	if !strings.HasSuffix(host, ".") {
		host += "."
	}
	h := sha256.Sum256([]byte(value))
	value = base64.RawURLEncoding.EncodeToString(h[:])
	payload := map[string]string{"host": host, "value": value}
	resp, err := c.postURL(addTXT, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while adding DNS-01 response for host %q, val %q (payload: %v): %w",
			host, value, payload, err,
		)
	}
	return resp, nil
}

// RemoveDNS01Response removes an ACME DNS-01 challenge response for the
// provided host from the challenge test server's DNS interfaces. Any failure
// returns an error that includes both the relevant operation and the payload.
func (c *Client) RemoveDNS01Response(host string) ([]byte, error) {
	if !strings.HasPrefix(host, "_acme-challenge.") {
		host = "_acme-challenge." + host
	}
	if !strings.HasSuffix(host, ".") {
		host += "."
	}
	payload := map[string]string{"host": host}
	resp, err := c.postURL(delTXT, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while removing DNS-01 response for host %q (payload: %v): %w",
			host, payload, err,
		)
	}
	return resp, nil
}

// DNSRequest is a single DNS request in the request history.
type DNSRequest struct {
	Question struct {
		Name   string `json:"Name"`
		Qtype  uint16 `json:"Qtype"`
		Qclass uint16 `json:"Qclass"`
	} `json:"Question"`
	UserAgent string `json:"UserAgent"`
}

// DNSRequestHistory returns the history of DNS requests made to the challenge
// test server's DNS interfaces for the given host. Any failure returns an error
// that includes both the relevant operation and the payload.
func (c *Client) DNSRequestHistory(host string) ([]DNSRequest, error) {
	payload := map[string]string{"host": host}
	raw, err := c.postURL(getDNSHistory, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while fetching DNS request history for host %q (payload: %v): %w",
			host, payload, err,
		)
	}
	var data []DNSRequest
	err = json.Unmarshal(raw, &data)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling DNS request history: %w", err)
	}
	return data, nil
}

// ClearDNSRequestHistory clears the history of DNS requests made to the
// challenge test server's DNS interfaces for the given host. Any failure
// returns an error that includes both the relevant operation and the payload.
func (c *Client) ClearDNSRequestHistory(host string) ([]byte, error) {
	resp, err := c.clearRequestHistory(host, "dns")
	if err != nil {
		return nil, fmt.Errorf(
			"while clearing DNS request history for host %q: %w", host, err,
		)
	}
	return resp, nil
}

// TLSALPN01Request is a single TLS-ALPN-01 request in the request history.
type TLSALPN01Request struct {
	ServerName      string   `json:"ServerName"`
	SupportedProtos []string `json:"SupportedProtos"`
}

// AddTLSALPN01Response adds an ACME TLS-ALPN-01 challenge response certificate
// to the challenge test server's TLS-ALPN-01 interface for the given host. The
// provided key authorization value will be embedded in the response certificate
// served to clients that initiate a TLS-ALPN-01 challenge validation with the
// challenge test server for the provided host. Any failure returns an error
// that includes both the relevant operation and the payload.
func (c *Client) AddTLSALPN01Response(host, value string) ([]byte, error) {
	payload := map[string]string{"host": host, "content": value}
	resp, err := c.postURL(addALPN, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while adding TLS-ALPN-01 response for host %q, val %q (payload: %v): %w",
			host, value, payload, err,
		)
	}
	return resp, nil
}

// RemoveTLSALPN01Response removes an ACME TLS-ALPN-01 challenge response
// certificate from the challenge test server's TLS-ALPN-01 interface for the
// given host. Any failure returns an error that includes both the relevant
// operation and the payload.
func (c *Client) RemoveTLSALPN01Response(host string) ([]byte, error) {
	payload := map[string]string{"host": host}
	resp, err := c.postURL(delALPN, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while removing TLS-ALPN-01 response for host %q (payload: %v): %w",
			host, payload, err,
		)
	}
	return resp, nil
}

// TLSALPN01RequestHistory returns the history of TLS-ALPN-01 requests made to
// the challenge test server's TLS-ALPN-01 interface for the given host. Any
// failure returns an error that includes both the relevant operation and the
// payload.
func (c *Client) TLSALPN01RequestHistory(host string) ([]TLSALPN01Request, error) {
	payload := map[string]string{"host": host}
	raw, err := c.postURL(getALPNHistory, payload)
	if err != nil {
		return nil, fmt.Errorf(
			"while fetching TLS-ALPN-01 request history for host %q (payload: %v): %w",
			host, payload, err,
		)
	}
	var data []TLSALPN01Request
	err = json.Unmarshal(raw, &data)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling TLS-ALPN-01 request history: %w", err)
	}
	return data, nil
}

// ClearTLSALPN01RequestHistory clears the history of TLS-ALPN-01 requests made
// to the challenge test server's TLS-ALPN-01 interface for the given host. Any
// failure returns an error that includes both the relevant operation and the
// payload.
func (c *Client) ClearTLSALPN01RequestHistory(host string) ([]byte, error) {
	resp, err := c.clearRequestHistory(host, "tlsalpn")
	if err != nil {
		return nil, fmt.Errorf(
			"while clearing TLS-ALPN-01 request history for host %q: %w", host, err,
		)
	}
	return resp, nil
}
