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
	bundlePath = test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0-bundle.json")
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
			name: "Prints output in JSON format",
			cli:  fmt.Sprintf("%s --format json", bundlePath),
			wants: Options{
				BundlePath:       bundlePath,
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

			assert.Equal(t, tc.wants.BundlePath, opts.BundlePath)
			assert.NotNil(t, opts.Logger)
			assert.Equal(t, tc.wantsExporter, opts.exporter != nil)
		})
	}
}

func TestRunInspect(t *testing.T) {
	opts := Options{
		BundlePath:       bundlePath,
		Logger:           io.NewTestHandler(),
		OCIClient:        oci.MockClient{},
		SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
	}

	t.Run("with valid bundle and default output", func(t *testing.T) {
		testIO, _, out, _ := iostreams.Test()
		opts.Logger = io.NewHandler(testIO)

		require.Nil(t, runInspect(&opts))
		outputStr := string(out.Bytes()[:])

		assert.Regexp(t, "PredicateType:......... https://slsa.dev/provenance/v1", outputStr)
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
		BundlePath:       bundlePath,
		Logger:           io.NewHandler(testIO),
		SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
		exporter:         cmdutil.NewJSONExporter(),
	}
	require.Nil(t, runInspect(&opts))

	var target BundleInspectResult
	err := json.Unmarshal(out.Bytes(), &target)

	assert.Equal(t, "https://github.com/sigstore/sigstore-js", target.InspectedBundles[0].Certificate.SourceRepositoryURI)
	assert.Equal(t, "https://slsa.dev/provenance/v1", target.InspectedBundles[0].Statement.PredicateType)
	require.NoError(t, err)
}
