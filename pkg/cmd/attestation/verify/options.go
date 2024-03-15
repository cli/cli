package verify

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
)

// Options captures the options for the verify command
type Options struct {
	ArtifactPath         string
	BundlePath           string
	CustomTrustedRoot    string
	DenySelfHostedRunner bool
	DigestAlgorithm      string
	JsonResult           bool
	NoPublicGood         bool
	OIDCIssuer           string
	Owner                string
	Quiet                bool
	Repo                 string
	SAN                  string
	SANRegex             string
	Verbose              bool
	APIClient            api.Client
	Logger               *io.Handler
	Limit                int
	OCIClient            oci.Client
}

// Clean cleans the file path option values
func (opts *Options) Clean() {
	if opts.BundlePath != "" {
		opts.BundlePath = filepath.Clean(opts.BundlePath)
	}
}

func (opts *Options) SetPolicyFlags() {
	// check that Repo is in the expected format if provided
	if opts.Repo != "" {
		// we expect the repo argument to be in the format <OWNER>/<REPO>
		splitRepo := strings.Split(opts.Repo, "/")

		// if Repo is provided but owner is not, set the OWNER portion of the Repo value
		// to Owner
		opts.Owner = splitRepo[0]

		if opts.SAN == "" && opts.SANRegex == "" {
			opts.SANRegex = expandToGitHubURL(opts.Repo)
		}
		return
	}
	if opts.SAN == "" && opts.SANRegex == "" {
		opts.SANRegex = expandToGitHubURL(opts.Owner)
	}
}

// AreFlagsValid checks that the provided flag combination is valid
// and returns an error otherwise
func (opts *Options) AreFlagsValid() error {
	// check that Repo is in the expected format if provided
	if opts.Repo != "" {
		// we expect the repo argument to be in the format <OWNER>/<REPO>
		splitRepo := strings.Split(opts.Repo, "/")
		if len(splitRepo) != 2 {
			return fmt.Errorf("invalid value provided for repo: %s", opts.Repo)
		}
	}

	// Check that limit is between 1 and 1000
	if opts.Limit < 1 || opts.Limit > 1000 {
		return fmt.Errorf("limit %d not allowed, must be between 1 and 1000", opts.Limit)
	}

	return nil
}

func expandToGitHubURL(ownerOrRepo string) string {
	return fmt.Sprintf("^https://github.com/%s/", ownerOrRepo)
}
