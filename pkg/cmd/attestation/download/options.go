package download

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
)

const (
	minLimit = 1
	maxLimit = 1000
)

type Options struct {
	APIClient       api.Client
	ArtifactPath    string
	DigestAlgorithm string
	Logger          *io.Handler
	Limit           int
	Store           MetadataStore
	OCIClient       oci.Client
	Owner           string
	PredicateType   string
	Repo            string
	Hostname        string
}

func (opts *Options) AreFlagsValid() error {
	// Check that limit is between 1 and 1000
	if opts.Limit < minLimit || opts.Limit > maxLimit {
		return fmt.Errorf("limit %d not allowed, must be between %d and %d", opts.Limit, minLimit, maxLimit)
	}

	return nil
}
