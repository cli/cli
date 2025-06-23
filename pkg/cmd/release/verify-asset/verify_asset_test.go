package verifyasset

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test/data"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cli/cli/v2/internal/ghrepo"
)

func TestNewCmdVerifyAsset_Args(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantTag  string
		wantFile string
		wantErr  string
	}{
		{
			name:     "valid args",
			args:     []string{"v1.2.3", "../../attestation/test/data/github_release_artifact.zip"},
			wantTag:  "v1.2.3",
			wantFile: test.NormalizeRelativePath("../../attestation/test/data/github_release_artifact.zip"),
		},
		{
			name: "valid flag with no tag",

			args:     []string{"../../attestation/test/data/github_release_artifact.zip"},
			wantFile: test.NormalizeRelativePath("../../attestation/test/data/github_release_artifact.zip"),
		},
		{
			name:    "no args",
			args:    []string{},
			wantErr: "you must specify an asset filepath",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testIO, _, _, _ := iostreams.Test()

			f := &cmdutil.Factory{
				IOStreams: testIO,
				HttpClient: func() (*http.Client, error) {
					return nil, nil
				},
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("owner/repo")
				},
			}

			var cfg *VerifyAssetConfig
			cmd := NewCmdVerifyAsset(f, func(c *VerifyAssetConfig) error {
				cfg = c
				return nil
			})
			cmd.SetArgs(tt.args)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err := cmd.ExecuteC()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantTag, cfg.Opts.TagName)
				assert.Equal(t, tt.wantFile, cfg.Opts.AssetFilePath)
			}
		})
	}
}

func Test_verifyAssetRun_Success(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	tagName := "v6"

	fakeHTTP := &httpmock.Registry{}
	defer fakeHTTP.Verify(t)

	fakeSHA := "1234567890abcdef1234567890abcdef12345678"
	shared.StubFetchRefSHA(t, fakeHTTP, "owner", "repo", tagName, fakeSHA)

	baseRepo, err := ghrepo.FromFullName("owner/repo")
	require.NoError(t, err)

	result := &verification.AttestationProcessingResult{
		Attestation: &api.Attestation{
			Bundle:    data.GitHubReleaseBundle(t),
			BundleURL: "https://example.com",
		},
		VerificationResult: nil,
	}

	releaseAssetPath := test.NormalizeRelativePath("../../attestation/test/data/github_release_artifact.zip")

	cfg := &VerifyAssetConfig{
		Opts: &VerifyAssetOptions{
			AssetFilePath: releaseAssetPath,
			TagName:       tagName,
			BaseRepo:      baseRepo,
			Exporter:      nil,
		},
		IO:          ios,
		HttpClient:  &http.Client{Transport: fakeHTTP},
		AttClient:   api.NewTestClient(),
		AttVerifier: shared.NewMockVerifier(result),
	}

	err = verifyAssetRun(cfg)
	require.NoError(t, err)
}

func Test_verifyAssetRun_FailedNoAttestations(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	tagName := "v1"

	fakeHTTP := &httpmock.Registry{}
	defer fakeHTTP.Verify(t)
	fakeSHA := "1234567890abcdef1234567890abcdef12345678"
	shared.StubFetchRefSHA(t, fakeHTTP, "owner", "repo", tagName, fakeSHA)

	baseRepo, err := ghrepo.FromFullName("owner/repo")
	require.NoError(t, err)

	releaseAssetPath := test.NormalizeRelativePath("../../attestation/test/data/github_release_artifact.zip")

	cfg := &VerifyAssetConfig{
		Opts: &VerifyAssetOptions{
			AssetFilePath: releaseAssetPath,
			TagName:       tagName,
			BaseRepo:      baseRepo,
			Exporter:      nil,
		},
		IO:          ios,
		HttpClient:  &http.Client{Transport: fakeHTTP},
		AttClient:   api.NewFailTestClient(),
		AttVerifier: nil,
	}

	err = verifyAssetRun(cfg)
	require.ErrorContains(t, err, "no attestations found for tag v1")
}

func Test_verifyAssetRun_FailedTagNotInAttestation(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	// Tag name does not match the one present in the attestation which
	// will be returned by the mock client. Simulates a scenario where
	// multiple releases may point to the same commit SHA, but not all
	// of them are attested.
	tagName := "v1.2.3"

	fakeHTTP := &httpmock.Registry{}
	defer fakeHTTP.Verify(t)
	fakeSHA := "1234567890abcdef1234567890abcdef12345678"
	shared.StubFetchRefSHA(t, fakeHTTP, "owner", "repo", tagName, fakeSHA)

	baseRepo, err := ghrepo.FromFullName("owner/repo")
	require.NoError(t, err)

	releaseAssetPath := test.NormalizeRelativePath("../../attestation/test/data/github_release_artifact.zip")

	cfg := &VerifyAssetConfig{
		Opts: &VerifyAssetOptions{
			AssetFilePath: releaseAssetPath,
			TagName:       tagName,
			BaseRepo:      baseRepo,
			Exporter:      nil,
		},
		IO:          ios,
		HttpClient:  &http.Client{Transport: fakeHTTP},
		AttClient:   api.NewTestClient(),
		AttVerifier: nil,
	}

	err = verifyAssetRun(cfg)
	require.ErrorContains(t, err, "no attestations found for release v1.2.3")
}

func Test_verifyAssetRun_FailedInvalidAsset(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	tagName := "v6"

	fakeHTTP := &httpmock.Registry{}
	defer fakeHTTP.Verify(t)
	fakeSHA := "1234567890abcdef1234567890abcdef12345678"
	shared.StubFetchRefSHA(t, fakeHTTP, "owner", "repo", tagName, fakeSHA)

	baseRepo, err := ghrepo.FromFullName("owner/repo")
	require.NoError(t, err)

	releaseAssetPath := test.NormalizeRelativePath("../../attestation/test/data/github_release_artifact_invalid.zip")

	cfg := &VerifyAssetConfig{
		Opts: &VerifyAssetOptions{
			AssetFilePath: releaseAssetPath,
			TagName:       tagName,
			BaseRepo:      baseRepo,
			Exporter:      nil,
		},
		IO:          ios,
		HttpClient:  &http.Client{Transport: fakeHTTP},
		AttClient:   api.NewTestClient(),
		AttVerifier: nil,
	}

	err = verifyAssetRun(cfg)
	require.ErrorContains(t, err, "attestation for v6 does not contain subject")
}

func Test_verifyAssetRun_NoSuchAsset(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	tagName := "v6"

	fakeHTTP := &httpmock.Registry{}
	fakeSHA := "1234567890abcdef1234567890abcdef12345678"
	shared.StubFetchRefSHA(t, fakeHTTP, "owner", "repo", tagName, fakeSHA)

	baseRepo, err := ghrepo.FromFullName("owner/repo")
	require.NoError(t, err)

	cfg := &VerifyAssetConfig{
		Opts: &VerifyAssetOptions{
			AssetFilePath: "artifact.zip",
			TagName:       tagName,
			BaseRepo:      baseRepo,
			Exporter:      nil,
		},
		IO:          ios,
		HttpClient:  &http.Client{Transport: fakeHTTP},
		AttClient:   api.NewTestClient(),
		AttVerifier: nil,
	}

	err = verifyAssetRun(cfg)
	require.ErrorContains(t, err, "failed to open local artifact")
}

func Test_getFileName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"foo/bar/baz.txt", "baz.txt"},
		{"baz.txt", "baz.txt"},
		{"/tmp/foo.tar.gz", "foo.tar.gz"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := getFileName(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
