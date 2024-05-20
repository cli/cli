package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	ioconfig "github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/go-gh/v2/pkg/auth"
)

const (
	DefaultLimit     = 30
	maxLimitForFlag  = 1000
	maxLimitForFetch = 100
)

type apiClient interface {
	RESTWithNext(hostname, method, p string, body io.Reader, data interface{}) (string, error)
}

type Client interface {
	GetByRepoAndDigest(repo, digest string, limit int) ([]*Attestation, error)
	GetByOwnerAndDigest(owner, digest string, limit int) ([]*Attestation, error)
}

type LiveClient struct {
	api    apiClient
	host   string
	logger *ioconfig.Handler
}

func NewLiveClient(hc *http.Client, l *ioconfig.Handler) *LiveClient {
	host, _ := auth.DefaultHost()

	return &LiveClient{
		api:    api.NewClientFromHTTP(hc),
		host:   strings.TrimSuffix(host, "/"),
		logger: l,
	}
}

func (c *LiveClient) BuildRepoAndDigestURL(repo, digest string) string {
	repo = strings.Trim(repo, "/")
	return fmt.Sprintf(GetAttestationByRepoAndSubjectDigestPath, repo, digest)
}

// GetByRepoAndDigest fetches the attestation by repo and digest
func (c *LiveClient) GetByRepoAndDigest(repo, digest string, limit int) ([]*Attestation, error) {
	url := c.BuildRepoAndDigestURL(repo, digest)
	return c.getAttestations(url, repo, digest, limit)
}

func (c *LiveClient) BuildOwnerAndDigestURL(owner, digest string) string {
	owner = strings.Trim(owner, "/")
	return fmt.Sprintf(GetAttestationByOwnerAndSubjectDigestPath, owner, digest)
}

// GetByOwnerAndDigest fetches attestation by owner and digest
func (c *LiveClient) GetByOwnerAndDigest(owner, digest string, limit int) ([]*Attestation, error) {
	url := c.BuildOwnerAndDigestURL(owner, digest)
	return c.getAttestations(url, owner, digest, limit)
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
	var err error
	// if no attestation or less than limit, then keep fetching
	for url != "" && len(attestations) < limit {
		url, err = c.api.RESTWithNext(c.host, http.MethodGet, url, nil, &resp)
		if err != nil {
			return nil, err
		}

		attestations = append(attestations, resp.Attestations...)
	}

	if len(attestations) == 0 {
		return nil, newErrNoAttestations(name, digest)
	}

	if len(attestations) > limit {
		return attestations[:limit], nil
	}

	return attestations, nil
}
