package download

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/digest"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/logger"
)

type Options struct {
	ArtifactPath    string
	DigestAlgorithm string
	APIClient    api.Client
	Logger          *logger.Logger
	Limit           int
	OCIClient       oci.Client
	OutputPath      string
	Owner           string
	Verbose         bool
}

// ConfigureLogger configures a logger using configuration provided
// through the options
func (opts *Options) ConfigureLogger() {
	opts.Logger = logger.NewLogger(false, opts.Verbose)
}

// ConfigureOCIClient configures an OCI client
func (opts *Options) ConfigureOCIClient() {
	opts.OCIClient = oci.NewLiveClient()
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
