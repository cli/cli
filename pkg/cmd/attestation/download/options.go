package download

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/digest"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/logging"
)

type Options struct {
	APIClient       api.Client
	ArtifactPath    string
	DigestAlgorithm string
	Logger          *logging.Logger
	Limit           int
	OCIClient       oci.Client
	OutputPath      string
	Owner           string
	Repo            string
	Verbose         bool
}

func (opts *Options) AreFlagsValid() error {
	if opts.Owner == "" {
		return fmt.Errorf("owner must be provided")
	}

	// DigestAlgorithm must not be empty
	if opts.DigestAlgorithm == "" {
		return fmt.Errorf("digest-alg cannot be empty")
	}

	if !digest.IsValidDigestAlgorithm(opts.DigestAlgorithm) {
		return fmt.Errorf("invalid digest algorithm '%s' provided in digest-alg", opts.DigestAlgorithm)
	}

	// Check that limit is between 1 and 1000
	if opts.Limit < 1 || opts.Limit > 1000 {
		return fmt.Errorf("limit %d not allowed, must be between 1 and 1000", opts.Limit)
	}

	return nil
}
