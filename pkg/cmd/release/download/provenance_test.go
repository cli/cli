package download

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	attapi "github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	in_toto "github.com/in-toto/attestation/go/v1"
	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_parseReleaseAssetURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantRepo  string
		wantHost  string
		wantTag   string
		wantAsset string
		wantErr   bool
	}{
		{
			name:      "github.com asset",
			url:       "https://github.com/vektra/mockery/releases/download/v2.53.8/mockery_2.53.8_Darwin_arm64.tar.gz",
			wantRepo:  "vektra/mockery",
			wantHost:  "github.com",
			wantTag:   "v2.53.8",
			wantAsset: "mockery_2.53.8_Darwin_arm64.tar.gz",
		},
		{
			name:      "tag containing a slash",
			url:       "https://github.com/OWNER/REPO/releases/download/pkg/v1.0.0/app.zip",
			wantRepo:  "OWNER/REPO",
			wantHost:  "github.com",
			wantTag:   "pkg/v1.0.0",
			wantAsset: "app.zip",
		},
		{
			name:      "escaped characters",
			url:       "https://github.com/OWNER/REPO/releases/download/v1%2B1/my%20app.zip",
			wantRepo:  "OWNER/REPO",
			wantHost:  "github.com",
			wantTag:   "v1+1",
			wantAsset: "my app.zip",
		},
		{
			name:      "tenant host",
			url:       "https://acme.ghe.com/OWNER/REPO/releases/download/v1/app.zip",
			wantRepo:  "OWNER/REPO",
			wantHost:  "acme.ghe.com",
			wantTag:   "v1",
			wantAsset: "app.zip",
		},
		{
			name:    "release page rather than asset",
			url:     "https://github.com/OWNER/REPO/releases/tag/v1",
			wantErr: true,
		},
		{
			name:    "missing asset",
			url:     "https://github.com/OWNER/REPO/releases/download/v1",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, tag, asset, err := parseReleaseAssetURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRepo, ghrepo.FullName(repo))
			assert.Equal(t, tt.wantHost, repo.RepoHost())
			assert.Equal(t, tt.wantTag, tag)
			assert.Equal(t, tt.wantAsset, asset)
		})
	}
}

func Test_NewCmdDownload_assetURLAndVerify(t *testing.T) {
	const assetURL = "https://github.com/OWNER/REPO/releases/download/v1.2.3/app.zip"
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "asset URL", args: []string{assetURL, "--verify"}},
		{name: "asset URL with pattern", args: []string{assetURL, "-p", "*"}, wantErr: "`--pattern` and `--archive` cannot be used with a release asset URL"},
		{name: "asset URL with repo", args: []string{assetURL, "-R", "a/b"}, wantErr: "`--repo` cannot be used with a release asset URL"},
		{name: "invalid asset URL", args: []string{"https://github.com/OWNER/REPO"}, wantErr: "expected a release asset URL"},
		{name: "verify with archive", args: []string{"v1", "--verify", "-A", "zip"}, wantErr: "`--verify` cannot be used with `--archive`"},
		{name: "verify to stdout", args: []string{"v1", "--verify", "-O", "-"}, wantErr: "`--verify` cannot be used when writing to standard output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			var opts *DownloadOptions
			cmd := NewCmdDownload(&cmdutil.Factory{IOStreams: ios}, func(o *DownloadOptions) error {
				opts = o
				return nil
			})
			cmd.PersistentFlags().StringP("repo", "R", "", "")
			cmd.SetArgs(tt.args)
			cmd.SetOut(&discard{})
			cmd.SetErr(&discard{})

			_, err := cmd.ExecuteC()
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.True(t, opts.Verify)
			assert.Equal(t, "v1.2.3", opts.TagName)
			assert.Equal(t, "app.zip", opts.AssetName)
			repo, err := opts.BaseRepo()
			require.NoError(t, err)
			assert.Equal(t, "OWNER/REPO", ghrepo.FullName(repo))
		})
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// sha256("hello")
const helloDigest = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

func verifiedResult(sourceRepoURI string) *verification.AttestationProcessingResult {
	return &verification.AttestationProcessingResult{
		Attestation: &attapi.Attestation{},
		VerificationResult: &verify.VerificationResult{
			Statement: &in_toto.Statement{PredicateType: verification.SLSAPredicateV1},
			Signature: &verify.SignatureVerificationResult{
				Certificate: &certificate.Summary{
					Issuer:                   verification.GitHubOIDCIssuer,
					SourceRepositoryURI:      sourceRepoURI,
					SourceRepositoryOwnerURI: sourceRepoURI[:strings.LastIndex(sourceRepoURI, "/")],
					SourceRepositoryDigest:   "abc123",
					SourceRepositoryRef:      "refs/tags/v1.2.3",
					BuildConfigURI:           sourceRepoURI + "/.github/workflows/release.yml@refs/tags/v1.2.3",
					BuildConfigDigest:        "abc123",
					BuildSignerURI:           sourceRepoURI + "/.github/workflows/release.yml@refs/tags/v1.2.3",
					RunInvocationURI:         sourceRepoURI + "/actions/runs/1/attempts/1",
					RunnerEnvironment:        "github-hosted",
				},
			},
		},
	}
}

type stubVerifier struct {
	results []*verification.AttestationProcessingResult
	err     error
}

func (v stubVerifier) Verify([]*attapi.Attestation, verify.PolicyBuilder) ([]*verification.AttestationProcessingResult, error) {
	return v.results, v.err
}

func Test_downloadRun_verify(t *testing.T) {
	foundAttestation := func(params attapi.FetchParams) ([]*attapi.Attestation, error) {
		assert.Equal(t, "sha256:"+helloDigest, params.Digest)
		assert.Equal(t, "OWNER/REPO", params.Repo)
		assert.Equal(t, verification.SLSAPredicateV1, params.PredicateType)
		return []*attapi.Attestation{{}}, nil
	}

	tests := []struct {
		name          string
		isTTY         bool
		immutable     bool
		body          string
		onGetByDigest func(attapi.FetchParams) ([]*attapi.Attestation, error)
		verifier      verification.SigstoreVerifier
		confirm       func(string, bool) (bool, error)
		wantErr       string
		wantStderr    []string
		wantFile      bool
	}{
		{
			name:          "verified provenance downloads when non-interactive",
			immutable:     true,
			body:          "hello",
			onGetByDigest: foundAttestation,
			verifier:      stubVerifier{results: []*verification.AttestationProcessingResult{verifiedResult("https://github.com/OWNER/REPO")}},
			wantStderr: []string{
				"OWNER/REPO v1.2.3",
				"✓ Immutable release",
				"✓ Verified SLSA build provenance",
				"Build instructions: https://github.com/OWNER/REPO/blob/abc123/.github/workflows/release.yml",
				"Source:             https://github.com/OWNER/REPO/tree/abc123",
				"Source ref:         refs/tags/v1.2.3",
				"Build run:          https://github.com/OWNER/REPO/actions/runs/1/attempts/1",
			},
			wantFile: true,
		},
		{
			name:          "missing provenance refuses when non-interactive",
			body:          "hello",
			onGetByDigest: func(attapi.FetchParams) ([]*attapi.Attestation, error) { return nil, attapi.ErrNoAttestationsFound },
			verifier:      stubVerifier{},
			wantErr:       "refusing to download: 1 asset lacking verified build provenance",
			wantStderr: []string{
				"! Not an immutable release",
				"X No build provenance attestation found",
			},
		},
		{
			name:          "attestation from another repository is rejected",
			body:          "hello",
			onGetByDigest: foundAttestation,
			verifier:      stubVerifier{results: []*verification.AttestationProcessingResult{verifiedResult("https://github.com/EVIL/REPO")}},
			wantErr:       "refusing to download",
			wantStderr:    []string{"X Attestation was not built from OWNER/REPO"},
		},
		{
			name:          "signature failure is rejected",
			body:          "hello",
			onGetByDigest: foundAttestation,
			verifier:      stubVerifier{err: errors.New("bad signature")},
			wantErr:       "refusing to download",
			wantStderr:    []string{"X Attestation signature verification failed: bad signature"},
		},
		{
			name:          "downloaded content not matching the attested digest is removed",
			body:          "tampered",
			onGetByDigest: foundAttestation,
			verifier:      stubVerifier{results: []*verification.AttestationProcessingResult{verifiedResult("https://github.com/OWNER/REPO")}},
			wantErr:       "does not match its attested digest",
		},
		{
			name:          "interactive decline cancels",
			isTTY:         true,
			body:          "hello",
			onGetByDigest: func(attapi.FetchParams) ([]*attapi.Attestation, error) { return nil, attapi.ErrNoAttestationsFound },
			verifier:      stubVerifier{},
			confirm: func(prompt string, def bool) (bool, error) {
				assert.Equal(t, "Download app.zip?", prompt)
				assert.False(t, def)
				return false, nil
			},
			wantErr: cmdutil.CancelError.Error(),
		},
		{
			name:          "interactive accept downloads despite missing provenance",
			isTTY:         true,
			body:          "hello",
			onGetByDigest: func(attapi.FetchParams) ([]*attapi.Attestation, error) { return nil, attapi.ErrNoAttestationsFound },
			verifier:      stubVerifier{},
			confirm:       func(string, bool) (bool, error) { return true, nil },
			wantFile:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			ios, _, _, stderr := iostreams.Test()
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStdoutTTY(tt.isTTY)

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			immutable := "false"
			if tt.immutable {
				immutable = "true"
			}
			shared.StubFetchRelease(t, reg, "OWNER", "REPO", "v1.2.3", `{
				"tag_name": "v1.2.3",
				"immutable": `+immutable+`,
				"assets": [
					{ "name": "app.zip", "digest": "sha256:`+helloDigest+`",
					  "url": "https://api.github.com/assets/1" },
					{ "name": "other.zip", "digest": "sha256:0000",
					  "url": "https://api.github.com/assets/2" }
				]
			}`)
			if tt.wantFile || tt.body != "hello" {
				reg.Register(httpmock.REST("GET", "assets/1"), httpmock.StringResponse(tt.body))
			}

			pm := &prompter.PrompterMock{ConfirmFunc: tt.confirm}
			opts := &DownloadOptions{
				IO:               ios,
				Prompter:         pm,
				HttpClient:       func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
				BaseRepo:         func() (ghrepo.Interface, error) { return ghrepo.FromFullName("OWNER/REPO") },
				TagName:          "v1.2.3",
				AssetName:        "app.zip",
				Destination:      dir,
				Concurrency:      1,
				Verify:           true,
				AttClient:        attapi.MockClient{OnGetByDigest: tt.onGetByDigest},
				SigstoreVerifier: tt.verifier,
			}

			err := downloadRun(opts)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			for _, s := range tt.wantStderr {
				assert.Contains(t, stderr.String(), s)
			}

			_, statErr := os.Stat(filepath.Join(dir, "app.zip"))
			if tt.wantFile {
				assert.NoError(t, statErr)
			} else {
				assert.True(t, os.IsNotExist(statErr), "expected app.zip not to exist")
			}
		})
	}
}
