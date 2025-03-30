package inspect

import (
	"path/filepath"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

// Options captures the options for the inspect command
type Options struct {
	ArtifactPath     string
	BundlePath       string
	DigestAlgorithm  string
	Logger           *io.Handler
	OCIClient        oci.Client
	SigstoreVerifier verification.SigstoreVerifier
	exporter         cmdutil.Exporter
	Hostname         string
	Tenant           string
}

// Clean cleans the file path option values
func (opts *Options) Clean() {
	opts.BundlePath = filepath.Clean(opts.BundlePath)
}
