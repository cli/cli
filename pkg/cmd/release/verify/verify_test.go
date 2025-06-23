package verify

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test/data"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdVerify_Args(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantTag string
		wantErr string
	}{
		{
			name:    "valid tag arg",
			args:    []string{"v1.2.3"},
			wantTag: "v1.2.3",
		},
		{
			name:    "no tag arg",
			args:    []string{},
			wantTag: "",
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

			var cfg *VerifyConfig
			cmd := NewCmdVerify(f, func(c *VerifyConfig) error {
				cfg = c
				return nil
			})
			cmd.SetArgs(tt.args)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err := cmd.ExecuteC()

			require.NoError(t, err)
			assert.Equal(t, tt.wantTag, cfg.Opts.TagName)
		})
	}
}

func Test_verifyRun_Success(t *testing.T) {
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

	cfg := &VerifyConfig{
		Opts: &VerifyOptions{
			TagName:  tagName,
			BaseRepo: baseRepo,
			Exporter: nil,
		},
		IO:          ios,
		HttpClient:  &http.Client{Transport: fakeHTTP},
		AttClient:   api.NewTestClient(),
		AttVerifier: shared.NewMockVerifier(result),
	}

	err = verifyRun(cfg)
	require.NoError(t, err)
}

func Test_verifyRun_FailedNoAttestations(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	tagName := "v1"

	fakeHTTP := &httpmock.Registry{}
	defer fakeHTTP.Verify(t)
	fakeSHA := "1234567890abcdef1234567890abcdef12345678"
	shared.StubFetchRefSHA(t, fakeHTTP, "owner", "repo", tagName, fakeSHA)

	baseRepo, err := ghrepo.FromFullName("owner/repo")
	require.NoError(t, err)

	cfg := &VerifyConfig{
		Opts: &VerifyOptions{
			TagName:  tagName,
			BaseRepo: baseRepo,
			Exporter: nil,
		},
		IO:          ios,
		HttpClient:  &http.Client{Transport: fakeHTTP},
		AttClient:   api.NewFailTestClient(),
		AttVerifier: nil,
	}

	err = verifyRun(cfg)
	require.ErrorContains(t, err, "no attestations for tag v1")
}

func Test_verifyRun_FailedTagNotInAttestation(t *testing.T) {
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

	cfg := &VerifyConfig{
		Opts: &VerifyOptions{
			TagName:  tagName,
			BaseRepo: baseRepo,
			Exporter: nil,
		},
		IO:          ios,
		HttpClient:  &http.Client{Transport: fakeHTTP},
		AttClient:   api.NewTestClient(),
		AttVerifier: nil,
	}

	err = verifyRun(cfg)
	require.ErrorContains(t, err, "no attestations found for release v1.2.3")
}
