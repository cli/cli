package api

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test/data"

	"github.com/stretchr/testify/require"
)

const (
	testRepo   = "github/example"
	testOwner  = "github"
	testDigest = "sha256:12313213"
)

func NewClientWithMockGHClient(hasNextPage bool) Client {
	fetcher := mockDataGenerator{
		NumAttestations: 5,
	}
	l := io.NewTestHandler()

	httpClient := &mockHttpClient{}

	if hasNextPage {
		return &LiveClient{
			githubAPI: mockAPIClient{
				OnRESTWithNext: fetcher.OnRESTSuccessWithNextPage,
			},
			httpClient: httpClient,
			logger:     l,
		}
	}

	return &LiveClient{
		githubAPI: mockAPIClient{
			OnRESTWithNext: fetcher.OnRESTSuccess,
		},
		httpClient: httpClient,
		logger:     l,
	}
}

func TestGetByDigest(t *testing.T) {
	c := NewClientWithMockGHClient(false)
	attestations, err := c.GetByRepoAndDigest(testRepo, testDigest, DefaultLimit)
	require.NoError(t, err)

	require.Equal(t, 5, len(attestations))
	bundle := (attestations)[0].Bundle
	require.Equal(t, bundle.GetMediaType(), "application/vnd.dev.sigstore.bundle.v0.3+json")

	attestations, err = c.GetByOwnerAndDigest(testOwner, testDigest, DefaultLimit)
	require.NoError(t, err)

	require.Equal(t, 5, len(attestations))
	bundle = (attestations)[0].Bundle
	require.Equal(t, bundle.GetMediaType(), "application/vnd.dev.sigstore.bundle.v0.3+json")
}

func TestGetByDigestGreaterThanLimit(t *testing.T) {
	c := NewClientWithMockGHClient(false)

	limit := 3
	// The method should return five results when the limit is not set
	attestations, err := c.GetByRepoAndDigest(testRepo, testDigest, limit)
	require.NoError(t, err)

	require.Equal(t, 3, len(attestations))
	bundle := (attestations)[0].Bundle
	require.Equal(t, bundle.GetMediaType(), "application/vnd.dev.sigstore.bundle.v0.3+json")

	attestations, err = c.GetByOwnerAndDigest(testOwner, testDigest, limit)
	require.NoError(t, err)

	require.Equal(t, len(attestations), limit)
	bundle = (attestations)[0].Bundle
	require.Equal(t, bundle.GetMediaType(), "application/vnd.dev.sigstore.bundle.v0.3+json")
}

func TestGetByDigestWithNextPage(t *testing.T) {
	c := NewClientWithMockGHClient(true)
	attestations, err := c.GetByRepoAndDigest(testRepo, testDigest, DefaultLimit)
	require.NoError(t, err)

	require.Equal(t, len(attestations), 10)
	bundle := (attestations)[0].Bundle
	require.Equal(t, bundle.GetMediaType(), "application/vnd.dev.sigstore.bundle.v0.3+json")

	attestations, err = c.GetByOwnerAndDigest(testOwner, testDigest, DefaultLimit)
	require.NoError(t, err)

	require.Equal(t, len(attestations), 10)
	bundle = (attestations)[0].Bundle
	require.Equal(t, bundle.GetMediaType(), "application/vnd.dev.sigstore.bundle.v0.3+json")
}

func TestGetByDigestGreaterThanLimitWithNextPage(t *testing.T) {
	c := NewClientWithMockGHClient(true)

	limit := 7
	// The method should return five results when the limit is not set
	attestations, err := c.GetByRepoAndDigest(testRepo, testDigest, limit)
	require.NoError(t, err)

	require.Equal(t, len(attestations), limit)
	bundle := (attestations)[0].Bundle
	require.Equal(t, bundle.GetMediaType(), "application/vnd.dev.sigstore.bundle.v0.3+json")

	attestations, err = c.GetByOwnerAndDigest(testOwner, testDigest, limit)
	require.NoError(t, err)

	require.Equal(t, len(attestations), limit)
	bundle = (attestations)[0].Bundle
	require.Equal(t, bundle.GetMediaType(), "application/vnd.dev.sigstore.bundle.v0.3+json")
}

func TestGetByDigest_NoAttestationsFound(t *testing.T) {
	fetcher := mockDataGenerator{
		NumAttestations: 5,
	}

	httpClient := &mockHttpClient{}
	c := LiveClient{
		githubAPI: mockAPIClient{
			OnRESTWithNext: fetcher.OnRESTWithNextNoAttestations,
		},
		httpClient: httpClient,
		logger:     io.NewTestHandler(),
	}

	attestations, err := c.GetByRepoAndDigest(testRepo, testDigest, DefaultLimit)
	require.Error(t, err)
	require.IsType(t, ErrNoAttestationsFound, err)
	require.Nil(t, attestations)

	attestations, err = c.GetByOwnerAndDigest(testOwner, testDigest, DefaultLimit)
	require.Error(t, err)
	require.IsType(t, ErrNoAttestationsFound, err)
	require.Nil(t, attestations)
}

func TestGetByDigest_Error(t *testing.T) {
	fetcher := mockDataGenerator{
		NumAttestations: 5,
	}

	c := LiveClient{
		githubAPI: mockAPIClient{
			OnRESTWithNext: fetcher.OnRESTWithNextError,
		},
		logger: io.NewTestHandler(),
	}

	attestations, err := c.GetByRepoAndDigest(testRepo, testDigest, DefaultLimit)
	require.Error(t, err)
	require.Nil(t, attestations)

	attestations, err = c.GetByOwnerAndDigest(testOwner, testDigest, DefaultLimit)
	require.Error(t, err)
	require.Nil(t, attestations)
}

func TestFetchBundleFromAttestations_BundleURL(t *testing.T) {
	httpClient := &mockHttpClient{}
	client := LiveClient{
		httpClient: httpClient,
		logger:     io.NewTestHandler(),
	}

	att1 := makeTestAttestation()
	att2 := makeTestAttestation()
	attestations := []*Attestation{&att1, &att2}
	fetched, err := client.fetchBundleFromAttestations(attestations)
	require.NoError(t, err)
	require.Len(t, fetched, 2)
	require.NotNil(t, "application/vnd.dev.sigstore.bundle.v0.3+json", fetched[0].Bundle.GetMediaType())
	httpClient.AssertNumberOfCalls(t, "OnGetSuccess", 2)
}

func TestFetchBundleFromAttestations_MissingBundleAndBundleURLFields(t *testing.T) {
	httpClient := &mockHttpClient{}
	client := LiveClient{
		httpClient: httpClient,
		logger:     io.NewTestHandler(),
	}

	// If both the BundleURL and Bundle fields are empty, the function should
	// return an error indicating that
	att1 := Attestation{}
	attestations := []*Attestation{&att1}
	bundles, err := client.fetchBundleFromAttestations(attestations)
	require.ErrorContains(t, err, "attestation has no bundle or bundle URL")
	require.Nil(t, bundles, 2)
}

func TestFetchBundleFromAttestations_FailOnTheSecondAttestation(t *testing.T) {
	mockHTTPClient := &failAfterNCallsHttpClient{
		// the initial HTTP request will succeed, which returns a bundle for the first attestation
		// all following HTTP requests will fail, which means the function fails to fetch a bundle
		// for the second attestation and the function returns an error
		FailOnCallN:              2,
		FailOnAllSubsequentCalls: true,
	}

	c := &LiveClient{
		httpClient: mockHTTPClient,
		logger:     io.NewTestHandler(),
	}

	att1 := makeTestAttestation()
	att2 := makeTestAttestation()
	attestations := []*Attestation{&att1, &att2}
	bundles, err := c.fetchBundleFromAttestations(attestations)
	require.Error(t, err)
	require.Nil(t, bundles)
}

func TestFetchBundleFromAttestations_FailAfterRetrying(t *testing.T) {
	mockHTTPClient := &reqFailHttpClient{}

	c := &LiveClient{
		httpClient: mockHTTPClient,
		logger:     io.NewTestHandler(),
	}

	a := makeTestAttestation()
	attestations := []*Attestation{&a}
	bundle, err := c.fetchBundleFromAttestations(attestations)
	require.Error(t, err)
	require.Nil(t, bundle)
	mockHTTPClient.AssertNumberOfCalls(t, "OnGetReqFail", 4)
}

func TestFetchBundleFromAttestations_FallbackToBundleField(t *testing.T) {
	mockHTTPClient := &mockHttpClient{}

	c := &LiveClient{
		httpClient: mockHTTPClient,
		logger:     io.NewTestHandler(),
	}

	// If the bundle URL is empty, the code will fallback to the bundle field
	a := Attestation{Bundle: data.SigstoreBundle(t)}
	attestations := []*Attestation{&a}
	fetched, err := c.fetchBundleFromAttestations(attestations)
	require.NoError(t, err)
	require.Equal(t, "application/vnd.dev.sigstore.bundle.v0.3+json", fetched[0].Bundle.GetMediaType())
	mockHTTPClient.AssertNotCalled(t, "OnGetSuccess")
}

// getBundle successfully fetches a bundle on the first HTTP request attempt
func TestGetBundle(t *testing.T) {
	mockHTTPClient := &mockHttpClient{}

	c := &LiveClient{
		httpClient: mockHTTPClient,
		logger:     io.NewTestHandler(),
	}

	b, err := c.getBundle("https://mybundleurl.com")
	require.NoError(t, err)
	require.Equal(t, "application/vnd.dev.sigstore.bundle.v0.3+json", b.GetMediaType())
	mockHTTPClient.AssertNumberOfCalls(t, "OnGetSuccess", 1)
}

// getBundle retries successfully when the initial HTTP request returns
// a 5XX status code
func TestGetBundle_SuccessfulRetry(t *testing.T) {
	mockHTTPClient := &failAfterNCallsHttpClient{
		FailOnCallN:              1,
		FailOnAllSubsequentCalls: false,
	}

	c := &LiveClient{
		httpClient: mockHTTPClient,
		logger:     io.NewTestHandler(),
	}

	b, err := c.getBundle("mybundleurl")
	require.NoError(t, err)
	require.Equal(t, "application/vnd.dev.sigstore.bundle.v0.3+json", b.GetMediaType())
	mockHTTPClient.AssertNumberOfCalls(t, "OnGetFailAfterNCalls", 2)
}

// getBundle does not retry when the function fails with a permanent backoff error condition
func TestGetBundle_PermanentBackoffFail(t *testing.T) {
	mockHTTPClient := &invalidBundleClient{}
	c := &LiveClient{
		httpClient: mockHTTPClient,
		logger:     io.NewTestHandler(),
	}

	b, err := c.getBundle("mybundleurl")
	// var permanent *backoff.PermanentError
	//require.IsType(t, &backoff.PermanentError{}, err)
	require.Error(t, err)
	require.Nil(t, b)
	mockHTTPClient.AssertNumberOfCalls(t, "OnGetInvalidBundle", 1)
}

// getBundle retries when the HTTP request fails
func TestGetBundle_RequestFail(t *testing.T) {
	mockHTTPClient := &reqFailHttpClient{}

	c := &LiveClient{
		httpClient: mockHTTPClient,
		logger:     io.NewTestHandler(),
	}

	b, err := c.getBundle("mybundleurl")
	require.Error(t, err)
	require.Nil(t, b)
	mockHTTPClient.AssertNumberOfCalls(t, "OnGetReqFail", 4)
}

func TestGetTrustDomain(t *testing.T) {
	fetcher := mockMetaGenerator{
		TrustDomain: "foo",
	}

	t.Run("with returned trust domain", func(t *testing.T) {
		c := LiveClient{
			githubAPI: mockAPIClient{
				OnREST: fetcher.OnREST,
			},
			logger: io.NewTestHandler(),
		}
		td, err := c.GetTrustDomain()
		require.Nil(t, err)
		require.Equal(t, "foo", td)

	})

	t.Run("with error", func(t *testing.T) {
		c := LiveClient{
			githubAPI: mockAPIClient{
				OnREST: fetcher.OnRESTError,
			},
			logger: io.NewTestHandler(),
		}
		td, err := c.GetTrustDomain()
		require.Equal(t, "", td)
		require.ErrorContains(t, err, "test error")
	})

}

func TestGetAttestationsRetries(t *testing.T) {
	getAttestationRetryInterval = 0

	fetcher := mockDataGenerator{
		NumAttestations: 5,
	}

	c := &LiveClient{
		githubAPI: mockAPIClient{
			OnRESTWithNext: fetcher.FlakyOnRESTSuccessWithNextPageHandler(),
		},
		httpClient: &mockHttpClient{},
		logger:     io.NewTestHandler(),
	}

	attestations, err := c.GetByRepoAndDigest(testRepo, testDigest, DefaultLimit)
	require.NoError(t, err)

	// assert the error path was executed; because this is a paged
	// request, it should have errored twice
	fetcher.AssertNumberOfCalls(t, "FlakyOnRESTSuccessWithNextPage:error", 2)

	// but we still successfully got the right data
	require.Equal(t, len(attestations), 10)
	bundle := (attestations)[0].Bundle
	require.Equal(t, bundle.GetMediaType(), "application/vnd.dev.sigstore.bundle.v0.3+json")

	// same test as above, but for GetByOwnerAndDigest:
	attestations, err = c.GetByOwnerAndDigest(testOwner, testDigest, DefaultLimit)
	require.NoError(t, err)

	// because we haven't reset the mock, we have added 2 more failed requests
	fetcher.AssertNumberOfCalls(t, "FlakyOnRESTSuccessWithNextPage:error", 4)

	require.Equal(t, len(attestations), 10)
	bundle = (attestations)[0].Bundle
	require.Equal(t, bundle.GetMediaType(), "application/vnd.dev.sigstore.bundle.v0.3+json")
}

// test total retries
func TestGetAttestationsMaxRetries(t *testing.T) {
	getAttestationRetryInterval = 0

	fetcher := mockDataGenerator{
		NumAttestations: 5,
	}

	c := &LiveClient{
		githubAPI: mockAPIClient{
			OnRESTWithNext: fetcher.OnREST500ErrorHandler(),
		},
		logger: io.NewTestHandler(),
	}

	_, err := c.GetByRepoAndDigest(testRepo, testDigest, DefaultLimit)
	require.Error(t, err)

	fetcher.AssertNumberOfCalls(t, "OnREST500Error", 4)
}
