package inspect

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	SigstoreSanValue = "https://github.com/sigstore/sigstore-js/.github/workflows/release.yml@refs/heads/main"
	SigstoreSanRegex = "^https://github.com/sigstore/sigstore-js/"
)

var (
	artifactPath = test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz")
	bundlePath   = test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0-bundle.json")
)

func TestNewInspectCmd(t *testing.T) {
	testIO, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: testIO,
		HttpClient: func() (*http.Client, error) {
			reg := &httpmock.Registry{}
			client := &http.Client{}
			httpmock.ReplaceTripper(client, reg)
			return client, nil
		},
	}

	testcases := []struct {
		name     string
		cli      string
		wants    Options
		wantsErr bool
	}{
		{
			name: "Invalid digest-alg flag",
			cli:  "../test/data/sigstore-js-2.1.0.tgz --bundle ../test/data/sigstore-js-2.1.0-bundle.json --digest-alg sha384",
			wants: Options{
				ArtifactPath:    "../test/data/sigstore-js-2.1.0.tgz",
				BundlePath:      "../test/data/sigstore-js-2.1.0-bundle.json",
				DigestAlgorithm: "sha384",
				OCIClient:       oci.MockClient{},
			},
			wantsErr: true,
		},
		{
			name: "Use default digest-alg value",
			cli:  "../test/data/sigstore-js-2.1.0.tgz --bundle ../test/data/sigstore-js-2.1.0-bundle.json",
			wants: Options{
				ArtifactPath:    "../test/data/sigstore-js-2.1.0.tgz",
				BundlePath:      "../test/data/sigstore-js-2.1.0-bundle.json",
				DigestAlgorithm: "sha256",
				OCIClient:       oci.MockClient{},
			},
			wantsErr: false,
		},
		{
			name: "Use custom digest-alg value",
			cli:  "../test/data/sigstore-js-2.1.0.tgz --bundle ../test/data/sigstore-js-2.1.0-bundle.json --digest-alg sha512",
			wants: Options{
				ArtifactPath:    "../test/data/sigstore-js-2.1.0.tgz",
				BundlePath:      "../test/data/sigstore-js-2.1.0-bundle.json",
				DigestAlgorithm: "sha512",
				OCIClient:       oci.MockClient{},
			},
			wantsErr: false,
		},
		{
			name: "Missing bundle flag",
			cli:  "../test/data/sigstore-js-2.1.0.tgz",
			wants: Options{
				ArtifactPath:    "../test/data/sigstore-js-2.1.0.tgz",
				DigestAlgorithm: "sha256",
				OCIClient:       oci.MockClient{},
			},
			wantsErr: true,
		},
		{
			name: "Has both verbose and quiet flags",
			cli:  "../test/data/sigstore-js-2.1.0.tgz --bundle ../test/data/sigstore-js-2.1.0-bundle.json --verbose --quiet",
			wants: Options{
				ArtifactPath:    "../test/data/sigstore-js-2.1.0.tgz",
				BundlePath:      "../test/data/sigstore-js-2.1.0-bundle.json",
				DigestAlgorithm: "sha256",
				OCIClient:       oci.MockClient{},
				Verbose:         true,
				Quiet:           true,
			},
			wantsErr: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var opts *Options
			cmd := NewInspectCmd(f, func(o *Options) error {
				opts = o
				return nil
			})

			argv, err := shlex.Split(tc.cli)
			assert.NoError(t, err)
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			_, err = cmd.ExecuteC()
			if tc.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tc.wants.ArtifactPath, opts.ArtifactPath)
			assert.Equal(t, tc.wants.BundlePath, opts.BundlePath)
			assert.Equal(t, tc.wants.DigestAlgorithm, opts.DigestAlgorithm)
			assert.Equal(t, tc.wants.JsonResult, opts.JsonResult)
			assert.Equal(t, tc.wants.Verbose, opts.Verbose)
			assert.Equal(t, tc.wants.Quiet, opts.Quiet)
		})
	}
}

func TestRunInspect(t *testing.T) {
	opts := Options{
		ArtifactPath:    artifactPath,
		BundlePath:      bundlePath,
		DigestAlgorithm: "sha512",
		Logger:          io.NewTestHandler(),
		OCIClient:       oci.MockClient{},
	}

	t.Run("with valid artifact and bundle", func(t *testing.T) {
		require.Nil(t, runInspect(&opts))
	})

	t.Run("with missing artifact path", func(t *testing.T) {
		customOpts := opts
		customOpts.ArtifactPath = "../test/data/non-existent-artifact.zip"
		require.Error(t, runInspect(&customOpts))
	})

	t.Run("with missing bundle path", func(t *testing.T) {
		customOpts := opts
		customOpts.BundlePath = "../test/data/non-existent-sigstoreBundle.json"
		require.Error(t, runInspect(&customOpts))
	})

	t.Run("with missing OCI client", func(t *testing.T) {
		customOpts := opts
		customOpts.ArtifactPath = "oci://ghcr.io/github/test"
		customOpts.OCIClient = nil
		require.Error(t, runInspect(&customOpts))
	})
}
