package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/cli/cli/v2/api"
	ioconfig "github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/golang/snappy"
	v1 "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	DefaultLimit     = 30
	maxLimitForFlag  = 1000
	maxLimitForFetch = 100
)

// Allow injecting backoff interval in tests.
var getAttestationRetryInterval = time.Millisecond * 200

// githubApiClient makes REST calls to the GitHub API
type githubApiClient interface {
	REST(hostname, method, p string, body io.Reader, data interface{}) error
	RESTWithNext(hostname, method, p string, body io.Reader, data interface{}) (string, error)
}

// httpClient makes HTTP calls to all non-GitHub API endpoints
type httpClient interface {
	Get(url string) (*http.Response, error)
}

type Client interface {
	GetByRepoAndDigest(repo, digest string, limit int) ([]*Attestation, error)
	GetByOwnerAndDigest(owner, digest string, limit int) ([]*Attestation, error)
	GetTrustDomain() (string, error)
}

type LiveClient struct {
	githubAPI  githubApiClient
	httpClient httpClient
	host       string
	logger     *ioconfig.Handler
}

func NewLiveClient(hc *http.Client, host string, l *ioconfig.Handler) *LiveClient {
	return &LiveClient{
		githubAPI:  api.NewClientFromHTTP(hc),
		host:       strings.TrimSuffix(host, "/"),
		httpClient: hc,
		logger:     l,
	}
}

func (c *LiveClient) BuildRepoAndDigestURL(repo, digest string) string {
	repo = strings.Trim(repo, "/")
	return fmt.Sprintf(GetAttestationByRepoAndSubjectDigestPath, repo, digest)
}

// GetByRepoAndDigest fetches the attestation by repo and digest
func (c *LiveClient) GetByRepoAndDigest(repo, digest string, limit int) ([]*Attestation, error) {
	url := c.BuildRepoAndDigestURL(repo, digest)
	attestations, err := c.getAttestations(url, repo, digest, limit)
	if err != nil {
		return nil, err
	}

	bundles, err := c.fetchBundleFromAttestations(attestations)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bundle with URL: %w", err)
	}

	return bundles, nil
}

func (c *LiveClient) BuildOwnerAndDigestURL(owner, digest string) string {
	owner = strings.Trim(owner, "/")
	return fmt.Sprintf(GetAttestationByOwnerAndSubjectDigestPath, owner, digest)
}

// GetByOwnerAndDigest fetches attestation by owner and digest
func (c *LiveClient) GetByOwnerAndDigest(owner, digest string, limit int) ([]*Attestation, error) {
	url := c.BuildOwnerAndDigestURL(owner, digest)
	attestations, err := c.getAttestations(url, owner, digest, limit)
	if err != nil {
		return nil, err
	}

	if len(attestations) == 0 {
		return nil, newErrNoAttestations(owner, digest)
	}

	bundles, err := c.fetchBundleFromAttestations(attestations)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bundle with URL: %w", err)
	}

	return bundles, nil
}

// GetTrustDomain returns the current trust domain. If the default is used
// the empty string is returned
func (c *LiveClient) GetTrustDomain() (string, error) {
	return c.getTrustDomain(MetaPath)
}

func (c *LiveClient) getAttestations(url, name, digest string, limit int) ([]*Attestation, error) {
	c.logger.VerbosePrintf("Fetching attestations for artifact digest %s\n\n", digest)

	perPage := limit
	if perPage <= 0 || perPage > maxLimitForFlag {
		return nil, fmt.Errorf("limit must be greater than 0 and less than or equal to %d", maxLimitForFlag)
	}

	if perPage > maxLimitForFetch {
		perPage = maxLimitForFetch
	}

	// ref: https://github.com/cli/go-gh/blob/d32c104a9a25c9de3d7c7b07a43ae0091441c858/example_gh_test.go#L96
	url = fmt.Sprintf("%s?per_page=%d", url, perPage)

	var attestations []*Attestation
	var resp AttestationsResponse
	bo := backoff.NewConstantBackOff(getAttestationRetryInterval)

	// if no attestation or less than limit, then keep fetching
	for url != "" && len(attestations) < limit {
		err := backoff.Retry(func() error {
			newURL, restErr := c.githubAPI.RESTWithNext(c.host, http.MethodGet, url, nil, &resp)

			if restErr != nil {
				if shouldRetry(restErr) {
					return restErr
				} else {
					return backoff.Permanent(restErr)
				}
			}

			url = newURL
			attestations = append(attestations, resp.Attestations...)

			return nil
		}, backoff.WithMaxRetries(bo, 3))

		// bail if RESTWithNext errored out
		if err != nil {
			return nil, err
		}
	}

	if len(attestations) == 0 {
		return nil, newErrNoAttestations(name, digest)
	}

	if len(attestations) > limit {
		return attestations[:limit], nil
	}

	return attestations, nil
}

func (c *LiveClient) fetchBundleFromAttestations(attestations []*Attestation) ([]*Attestation, error) {
	fetched := make([]*Attestation, len(attestations))
	g := errgroup.Group{}
	for i, a := range attestations {
		g.Go(func() error {
			if a.Bundle == nil && a.BundleURL == "" {
				return fmt.Errorf("attestation has no bundle or bundle URL")
			}

			// for now, we fallback to the bundle field if the bundle URL is empty
			if a.BundleURL == "" {
				c.logger.VerbosePrintf("Bundle URL is empty. Falling back to bundle field\n\n")
				fetched[i] = &Attestation{
					Bundle: a.Bundle,
				}
				return nil
			}

			// otherwise fetch the bundle with the provided URL
			b, err := c.GetBundle(a.BundleURL)
			if err != nil {
				return fmt.Errorf("failed to fetch bundle with URL: %w", err)
			}
			fetched[i] = &Attestation{
				Bundle: b,
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return fetched, nil
}

func (c *LiveClient) GetBundle(url string) (*bundle.Bundle, error) {
	c.logger.VerbosePrintf("Fetching attestation bundle with bundle URL\n\n")

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read blob storage response body: %w", err)
	}

	var out []byte
	decompressed, err := snappy.Decode(out, body)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress with snappy: %w", err)
	}

	var pbBundle v1.Bundle
	if err = protojson.Unmarshal(decompressed, &pbBundle); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to bundle: %w", err)
	}

	c.logger.VerbosePrintf("Successfully fetched bundle\n\n")

	return bundle.NewBundle(&pbBundle)
}

func shouldRetry(err error) bool {
	var httpError api.HTTPError
	if errors.As(err, &httpError) {
		if httpError.StatusCode >= 500 && httpError.StatusCode <= 599 {
			return true
		}
	}

	return false
}

func (c *LiveClient) getTrustDomain(url string) (string, error) {
	var resp MetaResponse

	bo := backoff.NewConstantBackOff(getAttestationRetryInterval)
	err := backoff.Retry(func() error {
		restErr := c.githubAPI.REST(c.host, http.MethodGet, url, nil, &resp)
		if restErr != nil {
			if shouldRetry(restErr) {
				return restErr
			} else {
				return backoff.Permanent(restErr)
			}
		}

		return nil
	}, backoff.WithMaxRetries(bo, 3))

	if err != nil {
		return "", err
	}

	return resp.Domains.ArtifactAttestations.TrustDomain, nil
}
