//go:build integration

package integration

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	berrors "github.com/letsencrypt/boulder/errors"
)

var ctSrvPorts = []int{4600, 4601, 4602, 4603, 4604, 4605, 4606, 4607, 4608, 4609}

// ctAddRejectHost adds a domain to all of the CT test server's reject-host
// lists. If this fails the test is aborted with a fatal error.
func ctAddRejectHost(domain string) error {
	for _, port := range ctSrvPorts {
		url := fmt.Sprintf("http://boulder.service.consul:%d/add-reject-host", port)
		body := []byte(fmt.Sprintf(`{"host": %q}`, domain))
		resp, err := http.Post(url, "", bytes.NewBuffer(body))
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("adding reject host: %d", resp.StatusCode)
		}
		resp.Body.Close()
	}
	return nil
}

// ctGetRejections returns a slice of base64 encoded certificates that were
// rejected by the CT test server at the specified port or an error.
func ctGetRejections(port int) ([]string, error) {
	url := fmt.Sprintf("http://boulder.service.consul:%d/get-rejections", port)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"getting rejections: status %d", resp.StatusCode)
	}
	var rejections []string
	err = json.NewDecoder(resp.Body).Decode(&rejections)
	if err != nil {
		return nil, err
	}
	return rejections, nil
}

// ctFindRejection returns a parsed x509.Certificate matching the given domains
// from the base64 certificates any CT test server rejected. If no rejected
// certificate matching the provided domains is found an error is returned.
func ctFindRejection(domains []string) (*x509.Certificate, error) {
	// Collect up rejections from all of the ctSrvPorts
	var rejections []string
	for _, port := range ctSrvPorts {
		r, err := ctGetRejections(port)
		if err != nil {
			continue
		}
		rejections = append(rejections, r...)
	}

	// Parse each rejection cert
	var cert *x509.Certificate
RejectionLoop:
	for _, r := range rejections {
		precertDER, err := base64.StdEncoding.DecodeString(r)
		if err != nil {
			return nil, err
		}
		c, err := x509.ParseCertificate(precertDER)
		if err != nil {
			return nil, err
		}
		// If the cert doesn't have the right number of names it won't be a match.
		if len(c.DNSNames) != len(domains) {
			continue
		}
		// If any names don't match, it isn't a match
		for i, name := range c.DNSNames {
			if name != domains[i] {
				continue RejectionLoop
			}
		}
		// It's a match!
		cert = c
		break
	}
	if cert == nil {
		return nil, berrors.NotFoundError("no matching ct-test-srv rejection found")
	}
	return cert, nil
}
