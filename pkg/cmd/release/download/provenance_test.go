package download

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	attapi "github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test/data"
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
	"google.golang.org/protobuf/types/known/structpb"
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

type stubReleaseVerifier struct {
	statement *in_toto.Statement
	err       error
}

func (v stubReleaseVerifier) VerifyAttestation(art *artifact.DigestedArtifact, _ *attapi.Attestation) (*verification.AttestationProcessingResult, error) {
	if art.DigestWithAlg() != "sha1:"+refSHA {
		return nil, errors.New("unexpected release ref " + art.DigestWithAlg())
	}
	if v.err != nil {
		return nil, v.err
	}
	return &verification.AttestationProcessingResult{
		VerificationResult: &verify.VerificationResult{Statement: v.statement},
	}, nil
}

const refSHA = "c5e17a62e06a1d201570249c61fae531e9244e1b"

func releaseStatement(t *testing.T, tag, releaseID string, subjects map[string]string) *in_toto.Statement {
	t.Helper()
	pred, err := structpb.NewStruct(map[string]any{"tag": tag, "databaseId": releaseID})
	require.NoError(t, err)
	st := &in_toto.Statement{PredicateType: "https://in-toto.io/attestation/release/v0.2", Predicate: pred}
	for name, digest := range subjects {
		st.Subject = append(st.Subject, &in_toto.ResourceDescriptor{Name: name, Digest: map[string]string{"sha256": digest}})
	}
	return st
}

func Test_downloadRun_verify(t *testing.T) {
	// The release attestation test bundle is for tag v6, so the release uses it too.
	const tag = "v6"
	goodProvenance := stubVerifier{results: []*verification.AttestationProcessingResult{verifiedResult("https://github.com/OWNER/REPO")}}
	noProvenance := stubVerifier{}
	goodRelease := func(t *testing.T) stubReleaseVerifier {
		return stubReleaseVerifier{statement: releaseStatement(t, tag, "42", map[string]string{"app.zip": helloDigest})}
	}

	tests := []struct {
		name          string
		isTTY         bool
		output        string
		immutable     bool
		hasReleaseAtt bool
		release       func(t *testing.T) stubReleaseVerifier
		hasProvenance bool
		verifier      verification.SigstoreVerifier
		confirm       func(string, bool) (bool, error)
		body          string // served for app.zip; empty means it must not be fetched
		wantErr       string
		wantStderr    []string
		wantStdout    string
		wantFile      bool
	}{
		{
			name:          "immutable release with verified attestations downloads when non-interactive",
			immutable:     true,
			hasReleaseAtt: true,
			release:       goodRelease,
			hasProvenance: true,
			verifier:      goodProvenance,
			body:          "hello",
			wantStderr: []string{
				"OWNER/REPO v6",
				"✓ Immutable release, verified by GitHub's signed release attestation",
				"✓ Listed in the signed release attestation",
				"✓ Verified SLSA build provenance",
				"Build instructions: https://github.com/OWNER/REPO/blob/abc123/.github/workflows/release.yml",
				"Source:             https://github.com/OWNER/REPO/tree/abc123",
				"Source ref:         refs/tags/v1.2.3",
				"Build run:          https://github.com/OWNER/REPO/actions/runs/1/attempts/1",
			},
			wantFile: true,
		},
		{
			name:          "mutable release with verified provenance downloads",
			hasProvenance: true,
			verifier:      goodProvenance,
			body:          "hello",
			wantStderr:    []string{"! Not an immutable release"},
			wantFile:      true,
		},
		{
			name:       "missing provenance refuses when non-interactive",
			verifier:   noProvenance,
			wantErr:    "refusing to download: 1 asset failed verification",
			wantStderr: []string{"X No build provenance attestation found"},
		},
		{
			name:          "provenance from another repository is rejected",
			hasProvenance: true,
			verifier:      stubVerifier{results: []*verification.AttestationProcessingResult{verifiedResult("https://github.com/EVIL/REPO")}},
			wantErr:       "refusing to download",
			wantStderr:    []string{"X Attestation was not built from OWNER/REPO"},
		},
		{
			name:          "provenance signature failure is rejected",
			hasProvenance: true,
			verifier:      stubVerifier{err: errors.New("bad signature")},
			wantErr:       "refusing to download",
			wantStderr:    []string{"X Attestation signature verification failed: bad signature"},
		},
		{
			name:          "immutable release without a release attestation is rejected",
			immutable:     true,
			hasProvenance: true,
			verifier:      goodProvenance,
			wantErr:       "refusing to download: the release attestation could not be verified",
			wantStderr:    []string{"X Release is marked immutable, but no signed release attestation found"},
		},
		{
			name:          "release attestation signature failure is rejected",
			immutable:     true,
			hasReleaseAtt: true,
			release:       func(*testing.T) stubReleaseVerifier { return stubReleaseVerifier{err: errors.New("bad signature")} },
			hasProvenance: true,
			verifier:      goodProvenance,
			wantErr:       "the release attestation could not be verified",
			wantStderr:    []string{"release attestation verification failed: bad signature"},
		},
		{
			name:          "release attestation for a different release is rejected",
			immutable:     true,
			hasReleaseAtt: true,
			release: func(t *testing.T) stubReleaseVerifier {
				return stubReleaseVerifier{statement: releaseStatement(t, tag, "99", map[string]string{"app.zip": helloDigest})}
			},
			hasProvenance: true,
			verifier:      goodProvenance,
			wantErr:       "the release attestation could not be verified",
			wantStderr:    []string{`release attestation is for release ID "99", not 42`},
		},
		{
			name:          "asset missing from the release attestation is rejected",
			immutable:     true,
			hasReleaseAtt: true,
			release: func(t *testing.T) stubReleaseVerifier {
				return stubReleaseVerifier{statement: releaseStatement(t, tag, "42", map[string]string{"other.zip": helloDigest})}
			},
			hasProvenance: true,
			verifier:      goodProvenance,
			wantErr:       "refusing to download: 1 asset failed verification",
			wantStderr:    []string{"X Not listed in the signed release attestation"},
		},
		{
			name:          "asset digest differing from the release attestation is rejected",
			immutable:     true,
			hasReleaseAtt: true,
			release: func(t *testing.T) stubReleaseVerifier {
				return stubReleaseVerifier{statement: releaseStatement(t, tag, "42", map[string]string{"app.zip": "ffff"})}
			},
			hasProvenance: true,
			verifier:      goodProvenance,
			wantErr:       "refusing to download: 1 asset failed verification",
			wantStderr:    []string{"X Digest differs from the signed release attestation (sha256:ffff)"},
		},
		{
			name:          "downloaded content not matching the attested digest is removed",
			hasProvenance: true,
			verifier:      goodProvenance,
			body:          "tampered",
			wantErr:       "does not match its attested digest",
		},
		{
			name:     "interactive decline cancels",
			isTTY:    true,
			verifier: noProvenance,
			confirm: func(prompt string, def bool) (bool, error) {
				assert.Equal(t, "Download app.zip?", prompt)
				assert.False(t, def)
				return false, nil
			},
			wantErr: cmdutil.CancelError.Error(),
		},
		{
			name:     "interactive accept downloads despite missing provenance",
			isTTY:    true,
			verifier: noProvenance,
			confirm:  func(string, bool) (bool, error) { return true, nil },
			body:     "hello",
			wantFile: true,
		},
		{
			name:     "interactive accept still checks the downloaded digest",
			isTTY:    true,
			verifier: noProvenance,
			confirm:  func(string, bool) (bool, error) { return true, nil },
			body:     "tampered",
			wantErr:  "does not match its attested digest",
		},
		{
			name:          "verified asset is written to standard output",
			output:        "-",
			hasProvenance: true,
			verifier:      goodProvenance,
			body:          "hello",
			wantStdout:    "hello",
		},
		{
			name:          "tampered asset is not written to standard output",
			output:        "-",
			hasProvenance: true,
			verifier:      goodProvenance,
			body:          "tampered",
			wantErr:       "does not match its attested digest",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStdoutTTY(tt.isTTY)

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.GraphQL(`query RepositoryReleaseByTag\b`), httpmock.StringResponse(`{ "data": { "repository": { "release": null }}}`))
			reg.Register(httpmock.REST("GET", "repos/OWNER/REPO/releases/tags/"+tag), httpmock.StringResponse(fmt.Sprintf(`{
				"id": 42,
				"tag_name": %q,
				"immutable": %t,
				"assets": [
					{ "name": "app.zip", "digest": "sha256:%s",
					  "url": "https://api.github.com/assets/1" },
					{ "name": "other.zip", "digest": "sha256:0000",
					  "url": "https://api.github.com/assets/2" }
				]
			}`, tag, tt.immutable, helloDigest)))
			if tt.immutable {
				shared.StubFetchRefSHA(t, reg, "OWNER", "REPO", tag, refSHA)
			}
			if tt.body != "" {
				reg.Register(httpmock.REST("GET", "assets/1"), httpmock.StringResponse(tt.body))
			}

			attClient := attapi.MockClient{OnGetByDigest: func(params attapi.FetchParams) ([]*attapi.Attestation, error) {
				assert.Equal(t, "OWNER/REPO", params.Repo)
				if params.PredicateType == "release" {
					assert.Equal(t, "sha1:"+refSHA, params.Digest)
					if !tt.hasReleaseAtt {
						return nil, attapi.ErrNoAttestationsFound
					}
					return []*attapi.Attestation{{Bundle: data.GitHubReleaseBundle(t)}}, nil
				}
				assert.Equal(t, "sha256:"+helloDigest, params.Digest)
				assert.Equal(t, verification.SLSAPredicateV1, params.PredicateType)
				if !tt.hasProvenance {
					return nil, attapi.ErrNoAttestationsFound
				}
				return []*attapi.Attestation{{}}, nil
			}}
			var releaseVerifier shared.Verifier = stubReleaseVerifier{err: errors.New("release verifier should not be called")}
			if tt.release != nil {
				releaseVerifier = tt.release(t)
			}

			opts := &DownloadOptions{
				IO:               ios,
				Prompter:         &prompter.PrompterMock{ConfirmFunc: tt.confirm},
				HttpClient:       func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
				BaseRepo:         func() (ghrepo.Interface, error) { return ghrepo.FromFullName("OWNER/REPO") },
				TagName:          tag,
				AssetName:        "app.zip",
				Destination:      dir,
				OutputFile:       tt.output,
				Concurrency:      1,
				Verify:           true,
				AttClient:        attClient,
				SigstoreVerifier: tt.verifier,
				ReleaseVerifier:  releaseVerifier,
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
			assert.Equal(t, tt.wantStdout, stdout.String())

			_, statErr := os.Stat(filepath.Join(dir, "app.zip"))
			if tt.wantFile {
				assert.NoError(t, statErr)
			} else {
				assert.True(t, os.IsNotExist(statErr), "expected app.zip not to exist")
			}
		})
	}
}

func Test_downloadRun_verifyUnsupportedHost(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	err := downloadRun(&DownloadOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.NewWithHost("OWNER", "REPO", "ghes.example.com"), nil
		},
		TagName:     "v1",
		Destination: t.TempDir(),
		Concurrency: 1,
		Verify:      true,
	})
	assert.EqualError(t, err, "`--verify` is not supported for ghes.example.com: artifact attestations are not available on GitHub Enterprise Server")
}
