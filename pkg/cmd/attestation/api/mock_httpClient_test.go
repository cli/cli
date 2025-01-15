package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/pkg/cmd/attestation/test/data"
	"github.com/golang/snappy"
	"github.com/stretchr/testify/mock"
)

type mockHttpClient struct {
	mock.Mock
}

func (m *mockHttpClient) Get(url string) (*http.Response, error) {
	m.On("OnGetSuccess").Return()
	m.MethodCalled("OnGetSuccess")

	var compressed []byte
	compressed = snappy.Encode(compressed, data.SigstoreBundleRaw)
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(compressed)),
	}, nil
}

type failHttpClient struct {
	mock.Mock
}

func (m *failHttpClient) Get(url string) (*http.Response, error) {
	m.On("OnGetFail").Return()
	m.MethodCalled("OnGetFail")

	return &http.Response{
		StatusCode: 500,
	}, fmt.Errorf("failed to fetch with %s", url)
}

type failAfterOneCallHttpClient struct {
	mock.Mock
}

func (m *failAfterOneCallHttpClient) Get(url string) (*http.Response, error) {
	m.On("OnGetFailAfterOneCall").Return()

	if len(m.Calls) >= 1 {
		m.MethodCalled("OnGetFailAfterOneCall")
		return &http.Response{
			StatusCode: 500,
		}, fmt.Errorf("failed to fetch with %s", url)
	}

	m.MethodCalled("OnGetFailAfterOneCall")
	var compressed []byte
	compressed = snappy.Encode(compressed, data.SigstoreBundleRaw)
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(compressed)),
	}, nil
}
