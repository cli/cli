package api

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

type mockAPIClient struct {
	OnREST func(hostname, method, p string, body io.Reader, data interface{}) error
	OnRESTWithNext func(hostname, method, p string, body io.Reader, data interface{}) (string, error) 
}

func (m mockAPIClient) REST(hostname, method, p string, body io.Reader, data interface{}) error {
	return m.OnREST(hostname, method, p, body, data)
}

func (m mockAPIClient) RESTWithNext(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	return m.OnRESTWithNext(hostname, method, p, body, data)
}

type mockDataGenerator struct {
	NumAttestations int
}

func (m mockDataGenerator) OnRESTSuccess(hostname, method, p string, body io.Reader, data interface{}) error {
	return m.OnRESTSuccessHelper(hostname, method, p, body, data, false)
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

func (m mockDataGenerator) OnRESTSuccessHelper(hostname, method, p string, body io.Reader, data interface{}, hasNext bool) error {
	atts := make([]*Attestation, m.NumAttestations)
	for j := 0; j < m.NumAttestations; j++ {
		att := makeTestAttestation()
		atts[j] = &att
	}

	var resp AttestationsResponse
	resp.Attestations = atts

	data = resp

	// // Convert the attestations to JSON
	// jsonResponse, err := json.Marshal(resp)
	// if err != nil {
	// 	return err
	// }

	// // Create a buffer containing the JSON response
	// responseReader := bytes.NewBuffer(jsonResponse)

	// linkHeader := ""
	// if hasNext {
	// 	// Create a link header with the next page
	// 	linkHeader = fmt.Sprintf("<%s&after=2>; rel=\"next\"", p)
	// }

	return nil
}

func (m mockDataGenerator) OnRESTWithNextSuccessHelper(hostname, method, p string, body io.Reader, data interface{}, hasNext bool) (string, error) {
	atts := make([]*Attestation, m.NumAttestations)
	for j := 0; j < m.NumAttestations; j++ {
		att := makeTestAttestation()
		atts[j] = &att
	}

	var resp AttestationsResponse
	resp.Attestations = atts

	data = resp

	// // Convert the attestations to JSON
	// jsonResponse, err := json.Marshal(resp)
	// if err != nil {
	// 	return err
	// }

	// // Create a buffer containing the JSON response
	// responseReader := bytes.NewBuffer(jsonResponse)
	
	// b, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	return "", err
	// }

	// err = json.Unmarshal(b, &data)
	// if err != nil {
	// 	return "", err
	// }


	linkHeader := ""
	if hasNext {
		// Create a link header with the next page
		linkHeader = fmt.Sprintf("<%s&after=2>; rel=\"next\"", p)
	}

	return linkHeader, nil
}

func (m mockDataGenerator) OnRESTNoAttestations(hostname, method, p string, body io.Reader, data interface{}) error {
	var resp AttestationsResponse
	resp.Attestations = make([]*Attestation, 0)

	data = resp

	// Convert the attestations to JSON
	// jsonResponse, err := json.Marshal(resp)
	// if err != nil {
	// 	return err
	// }

	// // Create a buffer containing the JSON response
	// responseReader := bytes.NewBuffer(jsonResponse)

	return nil
}

func (m mockDataGenerator) OnRESTWithNextNoAttestations(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	var resp AttestationsResponse
	resp.Attestations = make([]*Attestation, 0)

	data = resp

	// Convert the attestations to JSON
	// data, err := json.Marshal(resp)
	// if err != nil {
	// 	return err
	// }

	return "", nil
}

func (m mockDataGenerator) OnRESTError(hostname, method, p string, body io.Reader, data interface{}) error {
	return errors.New("failed to get attestations")
}

func (m mockDataGenerator) OnRESTWithNextError(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	return "", errors.New("failed to get attestations")
}
