package inspect

import (
	"path/filepath"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
)

// Options captures the options for the inspect command
type Options struct {
	ArtifactPath    string
	BundlePath      string
	DigestAlgorithm string
	JsonResult      bool
	Verbose         bool
	Quiet           bool
	Logger          *io.Handler
	OCIClient       oci.Client
}

// Clean cleans the file path option values
func (opts *Options) Clean() {
	opts.BundlePath = filepath.Clean(opts.BundlePath)
}
