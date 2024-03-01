package github

import "github.com/cli/cli/v2/api"

type Client interface {
	GetByRepoAndDigest(repo, digest string, limit int) ([]*Attestation, error)
	GetByOwnerAndDigest(owner, digest string, limit int) ([]*Attestation, error)
}

type LiveClient struct {
	apiClient api.Client
}

func NewLiveClient() (*LiveClient, error) {
	apiClient := api.NewClientFromHTTP(httpClient)
	return &LiveClient{
		apiClient: apiClient,
	}, nil
}
