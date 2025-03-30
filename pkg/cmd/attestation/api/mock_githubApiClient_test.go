package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	cliAPI "github.com/cli/cli/v2/api"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/mock"
)

type mockAPIClient struct {
	OnRESTWithNext func(hostname, method, p string, body io.Reader, data interface{}) (string, error)
	OnREST         func(hostname, method, p string, body io.Reader, data interface{}) error
}

func (m mockAPIClient) RESTWithNext(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	return m.OnRESTWithNext(hostname, method, p, body, data)
}

func (m mockAPIClient) REST(hostname, method, p string, body io.Reader, data interface{}) error {
	return m.OnREST(hostname, method, p, body, data)
}

type mockDataGenerator struct {
	mock.Mock
	NumAttestations int
}

func (m *mockDataGenerator) OnRESTSuccess(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	return m.OnRESTWithNextSuccessHelper(hostname, method, p, body, data, false)
}

func (m *mockDataGenerator) OnRESTSuccessWithNextPage(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	// if path doesn't contain after, it means first time hitting the mock server
	// so return the first page and return the link header in the response
	if !strings.Contains(p, "after") {
		return m.OnRESTWithNextSuccessHelper(hostname, method, p, body, data, true)
	}

	// if path contain after, it means second time hitting the mock server and will not return the link header
	return m.OnRESTWithNextSuccessHelper(hostname, method, p, body, data, false)
}

// Returns a func that just calls OnRESTSuccessWithNextPage but half the time
// it returns a 500 error.
func (m *mockDataGenerator) FlakyOnRESTSuccessWithNextPageHandler() func(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	// set up the flake counter
	m.On("FlakyOnRESTSuccessWithNextPage:error").Return()

	count := 0
	return func(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
		if count%2 == 0 {
			m.MethodCalled("FlakyOnRESTSuccessWithNextPage:error")

			count = count + 1
			return "", cliAPI.HTTPError{HTTPError: &ghAPI.HTTPError{StatusCode: 500}}
		} else {
			count = count + 1
			return m.OnRESTSuccessWithNextPage(hostname, method, p, body, data)
		}
	}
}

// always returns a 500
func (m *mockDataGenerator) OnREST500ErrorHandler() func(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	m.On("OnREST500Error").Return()
	return func(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
		m.MethodCalled("OnREST500Error")

		return "", cliAPI.HTTPError{HTTPError: &ghAPI.HTTPError{StatusCode: 500}}
	}
}

func (m *mockDataGenerator) OnRESTWithNextSuccessHelper(hostname, method, p string, body io.Reader, data interface{}, hasNext bool) (string, error) {
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

func (m *mockDataGenerator) OnRESTWithNextNoAttestations(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
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

func (m *mockDataGenerator) OnRESTWithNextError(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	return "", errors.New("failed to get attestations")
}

type mockMetaGenerator struct {
	TrustDomain string
}

func (m mockMetaGenerator) OnREST(hostname, method, p string, body io.Reader, data interface{}) error {
	var template = `
{
  "domains": {
    "artifact_attestations": {
      "trust_domain": "%s"
    }
  }
}
`
	var jsonString = fmt.Sprintf(template, m.TrustDomain)
	return json.Unmarshal([]byte(jsonString), &data)

}

func (m mockMetaGenerator) OnRESTError(hostname, method, p string, body io.Reader, data interface{}) error {
	return errors.New("test error")
}
