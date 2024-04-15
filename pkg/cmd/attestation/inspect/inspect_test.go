package inspect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"

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
		name          string
		cli           string
		wants         Options
		wantsErr      bool
		wantsExporter bool
	}{
		{
			name: "Invalid digest-alg flag",
			cli:  fmt.Sprintf("%s --bundle %s --digest-alg sha384", artifactPath, bundlePath),
			wants: Options{
				ArtifactPath:     artifactPath,
				BundlePath:       bundlePath,
				DigestAlgorithm:  "sha384",
				OCIClient:        oci.MockClient{},
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: true,
		},
		{
			name: "Use default digest-alg value",
			cli:  fmt.Sprintf("%s --bundle %s", artifactPath, bundlePath),
			wants: Options{
				ArtifactPath:     artifactPath,
				BundlePath:       bundlePath,
				DigestAlgorithm:  "sha256",
				OCIClient:        oci.MockClient{},
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: false,
		},
		{
			name: "Use custom digest-alg value",
			cli:  fmt.Sprintf("%s --bundle %s --digest-alg sha512", artifactPath, bundlePath),
			wants: Options{
				ArtifactPath:     artifactPath,
				BundlePath:       bundlePath,
				DigestAlgorithm:  "sha512",
				OCIClient:        oci.MockClient{},
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: false,
		},
		{
			name: "Missing bundle flag",
			cli:  artifactPath,
			wants: Options{
				ArtifactPath:     artifactPath,
				DigestAlgorithm:  "sha256",
				OCIClient:        oci.MockClient{},
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: true,
		},
		{
			name: "Prints output in JSON format",
			cli:  fmt.Sprintf("%s --bundle %s --format json", artifactPath, bundlePath),
			wants: Options{
				ArtifactPath:     artifactPath,
				BundlePath:       bundlePath,
				DigestAlgorithm:  "sha256",
				OCIClient:        oci.MockClient{},
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsExporter: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var opts *Options
			cmd := NewInspectCmd(f, func(o *Options) error {
				opts = o
				return nil
			})

			argv := strings.Split(tc.cli, " ")
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			_, err := cmd.ExecuteC()
			if tc.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tc.wants.ArtifactPath, opts.ArtifactPath)
			assert.Equal(t, tc.wants.BundlePath, opts.BundlePath)
			assert.Equal(t, tc.wants.DigestAlgorithm, opts.DigestAlgorithm)
			assert.NotNil(t, opts.Logger)
			assert.NotNil(t, opts.OCIClient)
			assert.Equal(t, tc.wantsExporter, opts.exporter != nil)
		})
	}
}

func TestRunInspect(t *testing.T) {
	opts := Options{
		ArtifactPath:     artifactPath,
		BundlePath:       bundlePath,
		DigestAlgorithm:  "sha512",
		Logger:           io.NewTestHandler(),
		OCIClient:        oci.MockClient{},
		SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
	}

	t.Run("with valid artifact and bundle", func(t *testing.T) {
		require.Nil(t, runInspect(&opts))
	})

	t.Run("with missing artifact path", func(t *testing.T) {
		customOpts := opts
		customOpts.ArtifactPath = test.NormalizeRelativePath("../test/data/non-existent-artifact.zip")
		require.Error(t, runInspect(&customOpts))
	})

	t.Run("with missing bundle path", func(t *testing.T) {
		customOpts := opts
		customOpts.BundlePath = test.NormalizeRelativePath("../test/data/non-existent-sigstoreBundle.json")
		require.Error(t, runInspect(&customOpts))
	})
}

func TestJSONOutput(t *testing.T) {
	testIO, _, out, _ := iostreams.Test()
	opts := Options{
		ArtifactPath:     artifactPath,
		BundlePath:       bundlePath,
		DigestAlgorithm:  "sha512",
		Logger:           io.NewHandler(testIO),
		OCIClient:        oci.MockClient{},
		SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
		exporter:         cmdutil.NewJSONExporter(),
	}
	require.Nil(t, runInspect(&opts))

	var target []AttestationDetail
	err := json.Unmarshal(out.Bytes(), &target)
	require.NoError(t, err)
}
