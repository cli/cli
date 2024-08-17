package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type mockAPIClient struct {
	OnRESTWithNext func(hostname, method, p string, body io.Reader, data interface{}) (string, error)
}

func (m mockAPIClient) RESTWithNext(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	return m.OnRESTWithNext(hostname, method, p, body, data)
}

type mockDataGenerator struct {
	NumAttestations int
}

func (m mockDataGenerator) OnRESTSuccess(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	return m.OnRESTWithNextSuccessHelper(hostname, method, p, body, data, false)
}

func (m mockDataGenerator) OnRESTSuccessWithNextPage(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	// if path doesn't contain after, it means first time hitting the mock server
	// so return the first page and return the link header in the response
	if !strings.Contains(p, "after") {
		return m.OnRESTWithNextSuccessHelper(hostname, method, p, body, data, true)
	}

	// if path contain after, it means second time hitting the mock server and will not return the link header
	return m.OnRESTWithNextSuccessHelper(hostname, method, p, body, data, false)
}

func (m mockDataGenerator) OnRESTWithNextSuccessHelper(hostname, method, p string, body io.Reader, data interface{}, hasNext bool) (string, error) {
	atts := make([]*Attestation, m.NumAttestations)
	for j := 0; j < m.NumAttestations; j++ {
		att := makeTestAttestation()
		atts[j] = &att
	}

	resp := AttestationsResponse{
		Attestations: atts,
	}

	// // Convert the attestations to JSON
	b, err := json.Marshal(resp)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(b, &data)
	if err != nil {
		return "", err
	}

	if hasNext {
		// return a link header with the next page
		return fmt.Sprintf("<%s&after=2>; rel=\"next\"", p), nil
	}

	return "", nil
}

func (m mockDataGenerator) OnRESTWithNextNoAttestations(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	resp := AttestationsResponse{
		Attestations: make([]*Attestation, 0),
	}

	// // Convert the attestations to JSON
	b, err := json.Marshal(resp)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(b, &data)
	if err != nil {
		return "", err
	}

	return "", nil
}

func (m mockDataGenerator) OnRESTWithNextError(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	return "", errors.New("failed to get attestations")
}
