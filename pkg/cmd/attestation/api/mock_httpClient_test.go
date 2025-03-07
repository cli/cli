package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"

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

type invalidBundleClient struct {
	mock.Mock
}

func (m *invalidBundleClient) Get(url string) (*http.Response, error) {
	m.On("OnGetInvalidBundle").Return()
	m.MethodCalled("OnGetInvalidBundle")

	var compressed []byte
	compressed = snappy.Encode(compressed, []byte("invalid bundle bytes"))
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(compressed)),
	}, nil
}

type reqFailHttpClient struct {
	mock.Mock
}

func (m *reqFailHttpClient) Get(url string) (*http.Response, error) {
	m.On("OnGetReqFail").Return()
	m.MethodCalled("OnGetReqFail")

	return &http.Response{
		StatusCode: 500,
	}, fmt.Errorf("failed to fetch with %s", url)
}

type failAfterNCallsHttpClient struct {
	mock.Mock
	mu                       sync.Mutex
	FailOnCallN              int
	FailOnAllSubsequentCalls bool
	NumCalls                 int
}

func (m *failAfterNCallsHttpClient) Get(url string) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.On("OnGetFailAfterNCalls").Return()

	m.NumCalls++

	if m.NumCalls == m.FailOnCallN || (m.NumCalls > m.FailOnCallN && m.FailOnAllSubsequentCalls) {
		m.MethodCalled("OnGetFailAfterNCalls")
		return &http.Response{
			StatusCode: 500,
		}, nil
	}

	m.MethodCalled("OnGetFailAfterNCalls")
	var compressed []byte
	compressed = snappy.Encode(compressed, data.SigstoreBundleRaw)
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(compressed)),
	}, nil
}
