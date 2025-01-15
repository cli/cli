package download

import (
	"bytes"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var artifactPath = test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz")

func expectedFilePath(tempDir string, digestWithAlg string) string {
	var filename string
	if runtime.GOOS == "windows" {
		filename = fmt.Sprintf("%s.jsonl", strings.ReplaceAll(digestWithAlg, ":", "-"))
	} else {
		filename = fmt.Sprintf("%s.jsonl", digestWithAlg)
	}

	return test.NormalizeRelativePath(fmt.Sprintf("%s/%s", tempDir, filename))
}

func TestNewDownloadCmd(t *testing.T) {
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

	store := &LiveStore{
		outputPath: t.TempDir(),
	}

	testcases := []struct {
		name     string
		cli      string
		wants    Options
		wantsErr bool
	}{
		{
			name: "Invalid digest-alg flag",
			cli:  fmt.Sprintf("%s --owner sigstore --digest-alg sha384", artifactPath),
			wants: Options{
				ArtifactPath:    artifactPath,
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha384",
				Owner:           "sigstore",
				Store:           store,
				Limit:           30,
			},
			wantsErr: true,
		},
		{
			name: "Missing digest-alg flag",
			cli:  fmt.Sprintf("%s --owner sigstore", artifactPath),
			wants: Options{
				ArtifactPath:    artifactPath,
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha256",
				Owner:           "sigstore",
				Store:           store,
				Limit:           30,
			},
			wantsErr: false,
		},
		{
			name: "Missing owner and repo flags",
			cli:  artifactPath,
			wants: Options{
				ArtifactPath:    artifactPath,
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha256",
				Owner:           "sigstore",
				Store:           store,
				Limit:           30,
			},
			wantsErr: true,
		},
		{
			name: "Has both owner and repo flags",
			cli:  fmt.Sprintf("%s --owner sigstore --repo sigstore/sigstore-js", artifactPath),
			wants: Options{
				ArtifactPath:    artifactPath,
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha256",
				Owner:           "sigstore",
				Store:           store,
				Repo:            "sigstore/sigstore-js",
				Limit:           30,
			},
			wantsErr: true,
		},
		{
			name: "Uses default limit flag",
			cli:  fmt.Sprintf("%s --owner sigstore", artifactPath),
			wants: Options{
				ArtifactPath:    artifactPath,
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha256",
				Owner:           "sigstore",
				Store:           store,
				Limit:           30,
			},
			wantsErr: false,
		},
		{
			name: "Uses custom limit flag",
			cli:  fmt.Sprintf("%s --owner sigstore --limit 101", artifactPath),
			wants: Options{
				ArtifactPath:    artifactPath,
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha256",
				Owner:           "sigstore",
				Store:           store,
				Limit:           101,
			},
			wantsErr: false,
		},
		{
			name: "Uses invalid limit flag",
			cli:  fmt.Sprintf("%s --owner sigstore --limit 0", artifactPath),
			wants: Options{
				ArtifactPath:    artifactPath,
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha256",
				Owner:           "sigstore",
				Store:           store,
				Limit:           0,
			},
			wantsErr: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var opts *Options
			cmd := NewDownloadCmd(f, func(o *Options) error {
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

			assert.Equal(t, tc.wants.DigestAlgorithm, opts.DigestAlgorithm)
			assert.Equal(t, tc.wants.Limit, opts.Limit)
			assert.Equal(t, tc.wants.Owner, opts.Owner)
			assert.Equal(t, tc.wants.Repo, opts.Repo)
			assert.NotNil(t, opts.APIClient)
			assert.NotNil(t, opts.OCIClient)
			assert.NotNil(t, opts.Logger)
			assert.NotNil(t, opts.Store)
		})
	}
}

func TestRunDownload(t *testing.T) {
	tempDir := t.TempDir()
	store := &LiveStore{
		outputPath: tempDir,
	}

	baseOpts := Options{
		ArtifactPath:    artifactPath,
		APIClient:       api.NewTestClient(),
		OCIClient:       oci.MockClient{},
		DigestAlgorithm: "sha512",
		Owner:           "sigstore",
		Store:           store,
		Limit:           30,
		Logger:          io.NewTestHandler(),
	}

	t.Run("fetch and store attestations successfully with owner", func(t *testing.T) {
		err := runDownload(&baseOpts)
		require.NoError(t, err)

		artifact, err := artifact.NewDigestedArtifact(baseOpts.OCIClient, baseOpts.ArtifactPath, baseOpts.DigestAlgorithm)
		require.NoError(t, err)

		expectedFilePath := expectedFilePath(tempDir, artifact.DigestWithAlg())
		require.FileExists(t, expectedFilePath)

		actualLineCount, err := countLines(expectedFilePath)
		require.NoError(t, err)

		expectedLineCount := 2
		require.Equal(t, expectedLineCount, actualLineCount)
	})

	t.Run("fetch and store attestations successfully with repo", func(t *testing.T) {
		opts := baseOpts
		opts.Owner = ""
		opts.Repo = "sigstore/sigstore-js"

		err := runDownload(&opts)
		require.NoError(t, err)

		artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
		require.NoError(t, err)

		expectedFilePath := expectedFilePath(tempDir, artifact.DigestWithAlg())
		require.FileExists(t, expectedFilePath)

		actualLineCount, err := countLines(expectedFilePath)
		require.NoError(t, err)

		expectedLineCount := 2
		require.Equal(t, expectedLineCount, actualLineCount)
	})

	t.Run("download OCI image attestations successfully", func(t *testing.T) {
		opts := baseOpts
		opts.ArtifactPath = "oci://ghcr.io/github/test"

		err := runDownload(&opts)
		require.NoError(t, err)

		artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
		require.NoError(t, err)

		expectedFilePath := expectedFilePath(tempDir, artifact.DigestWithAlg())
		require.FileExists(t, expectedFilePath)

		actualLineCount, err := countLines(expectedFilePath)
		require.NoError(t, err)

		expectedLineCount := 2
		require.Equal(t, expectedLineCount, actualLineCount)
	})

	t.Run("cannot find artifact", func(t *testing.T) {
		opts := baseOpts
		opts.ArtifactPath = "../test/data/not-real.zip"

		err := runDownload(&opts)
		require.Error(t, err)
	})

	t.Run("no attestations found", func(t *testing.T) {
		opts := baseOpts
		opts.APIClient = api.MockClient{
			OnGetByOwnerAndDigest: func(repo, digest string, limit int) ([]*api.Attestation, error) {
				return nil, api.ErrNoAttestations{}
			},
		}

		err := runDownload(&opts)
		require.NoError(t, err)

		artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
		require.NoError(t, err)
		require.NoFileExists(t, artifact.DigestWithAlg())
	})

	t.Run("failed to fetch attestations", func(t *testing.T) {
		opts := baseOpts
		opts.APIClient = api.MockClient{
			OnGetByOwnerAndDigest: func(repo, digest string, limit int) ([]*api.Attestation, error) {
				return nil, fmt.Errorf("failed to fetch attestations")
			},
		}

		err := runDownload(&opts)
		require.Error(t, err)
	})

	t.Run("cannot download OCI artifact", func(t *testing.T) {
		opts := baseOpts
		opts.ArtifactPath = "oci://ghcr.io/github/test"
		opts.OCIClient = oci.ReferenceFailClient{}

		err := runDownload(&opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to digest artifact")
	})

	t.Run("with missing API client", func(t *testing.T) {
		customOpts := baseOpts
		customOpts.APIClient = nil
		require.Error(t, runDownload(&customOpts))
	})

	t.Run("fail to write attestations to metadata file", func(t *testing.T) {
		opts := baseOpts
		opts.Store = &MockStore{
			OnCreateMetadataFile: OnCreateMetadataFileFailure,
		}

		err := runDownload(&opts)
		require.Error(t, err)
		require.ErrorAs(t, err, &ErrAttestationFileCreation)
	})
}
